const std = @import("std");
const apply = @import("apply");

const original = "{\n  \"enabled\": false,\n  \"keep\": \"a\\u0041\"\n}\n";
const expected = "{\n  \"enabled\": true,\n  \"keep\": \"a\\u0041\"\n}\n";

fn sourceDigest() [71]u8 {
    var hash: [32]u8 = undefined;
    std.crypto.hash.sha2.Sha256.hash(original, &hash, .{});
    var value: [71]u8 = undefined;
    @memcpy(value[0..7], "sha256:");
    const hex = std.fmt.bytesToHex(hash, .lower);
    @memcpy(value[7..], &hex);
    return value;
}

fn readAll(allocator: std.mem.Allocator, dir: std.Io.Dir, path: []const u8) ![]u8 {
    var file = try dir.openFile(std.testing.io, path, .{});
    defer file.close(std.testing.io);
    var buffer: [256]u8 = undefined;
    var reader = file.reader(std.testing.io, &buffer);
    return reader.interface.allocRemaining(allocator, .unlimited);
}

test "atomic apply preserves permissions and writes recovery" {
    var tmp = std.testing.tmpDir(.{});
    defer tmp.cleanup();
    const permissions: std.Io.File.Permissions = @enumFromInt(0o640);
    try tmp.dir.writeFile(std.testing.io, .{ .sub_path = "app.json", .data = original, .flags = .{ .permissions = permissions } });
    const initial_stat = try tmp.dir.statFile(std.testing.io, "app.json", .{});
    const digest = sourceDigest();
    const edits = [_]apply.Edit{.{ .path = "/enabled", .operation = .replace, .value = "true" }};
    const result = try apply.applyInDir(std.testing.allocator, std.testing.io, tmp.dir, "app.json", &digest, &edits);
    try std.testing.expect(result.recovery_available);
    const actual = try readAll(std.testing.allocator, tmp.dir, "app.json");
    defer std.testing.allocator.free(actual);
    try std.testing.expectEqualStrings(expected, actual);
    const recovery = try readAll(std.testing.allocator, tmp.dir, "app.json.zconfig-recovery");
    defer std.testing.allocator.free(recovery);
    try std.testing.expectEqualStrings(original, recovery);
    const stat = try tmp.dir.statFile(std.testing.io, "app.json", .{});
    try std.testing.expectEqual(initial_stat.permissions, stat.permissions);
}

test "stale digest and injected failures never partially change source" {
    const faults = [_]apply.Fault{ .temp_creation, .write, .flush, .permission, .recovery, .replacement, .locked, .interrupt_after_recovery };
    const edits = [_]apply.Edit{.{ .path = "/enabled", .operation = .replace, .value = "true" }};
    for (faults) |fault| {
        var tmp = std.testing.tmpDir(.{});
        defer tmp.cleanup();
        try tmp.dir.writeFile(std.testing.io, .{ .sub_path = "app.json", .data = original });
        const digest = sourceDigest();
        try std.testing.expectError(switch (fault) {
            .temp_creation => error.InjectedTempCreationFailure,
            .write => error.InjectedWriteFailure,
            .flush => error.InjectedFlushFailure,
            .permission => error.InjectedPermissionFailure,
            .recovery => error.InjectedRecoveryFailure,
            .replacement => error.InjectedReplacementFailure,
            .locked => error.LockedDestination,
            .interrupt_after_recovery => error.Interrupted,
            .none => unreachable,
        }, apply.applyInDirWithFault(std.testing.allocator, std.testing.io, tmp.dir, "app.json", &digest, &edits, fault));
        const actual = try readAll(std.testing.allocator, tmp.dir, "app.json");
        defer std.testing.allocator.free(actual);
        try std.testing.expectEqualStrings(original, actual);
        if (fault == .temp_creation or fault == .write or fault == .flush or fault == .replacement or fault == .locked or fault == .interrupt_after_recovery) {
            const recovery = try readAll(std.testing.allocator, tmp.dir, "app.json.zconfig-recovery");
            defer std.testing.allocator.free(recovery);
            try std.testing.expectEqualStrings(original, recovery);
        }
    }

    var tmp = std.testing.tmpDir(.{});
    defer tmp.cleanup();
    try tmp.dir.writeFile(std.testing.io, .{ .sub_path = "app.json", .data = original });
    const stale = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb";
    try std.testing.expectError(error.StaleSource, apply.applyInDir(std.testing.allocator, std.testing.io, tmp.dir, "app.json", stale, &edits));
}

test "unsupported file kinds fail closed before mutation" {
    var tmp = std.testing.tmpDir(.{});
    defer tmp.cleanup();
    const digest = sourceDigest();
    const edits = [_]apply.Edit{.{ .path = "/enabled", .operation = .replace, .value = "true" }};
    try tmp.dir.createDir(std.testing.io, "directory", .default_dir);
    try std.testing.expectError(error.NotRegularFile, apply.applyInDir(std.testing.allocator, std.testing.io, tmp.dir, "directory", &digest, &edits));

    try tmp.dir.writeFile(std.testing.io, .{ .sub_path = "target.json", .data = original });
    try tmp.dir.symLink(std.testing.io, "target.json", "link.json", .{});
    try std.testing.expectError(error.NotRegularFile, apply.applyInDir(std.testing.allocator, std.testing.io, tmp.dir, "link.json", &digest, &edits));
    const target = try readAll(std.testing.allocator, tmp.dir, "target.json");
    defer std.testing.allocator.free(target);
    try std.testing.expectEqualStrings(original, target);
}
