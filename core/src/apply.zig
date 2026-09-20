const std = @import("std");
const source_index = @import("source_index.zig");
const pointer = @import("pointer.zig");

const max_source_bytes: usize = 10 * 1024 * 1024;

pub const Operation = enum { add, replace, remove };
pub const Edit = struct { path: []const u8, operation: Operation, value: ?[]const u8 = null };

const Patch = struct { start: usize, end: usize, replacement: []u8, order: usize };

pub const Fault = enum { none, temp_creation, write, flush, permission, recovery, replacement, locked, interrupt_after_recovery };

pub const ApplyResult = struct {
    source_digest_before: [71]u8,
    source_digest_after: [71]u8,
    recovery_available: bool,
};

pub fn applyInDir(
    allocator: std.mem.Allocator,
    io: std.Io,
    dir: std.Io.Dir,
    sub_path: []const u8,
    expected_digest: []const u8,
    edits: []const Edit,
) !ApplyResult {
    return applyInDirWithFault(allocator, io, dir, sub_path, expected_digest, edits, .none);
}

pub fn applyInDirWithFault(
    allocator: std.mem.Allocator,
    io: std.Io,
    dir: std.Io.Dir,
    sub_path: []const u8,
    expected_digest: []const u8,
    edits: []const Edit,
    fault: Fault,
) !ApplyResult {
    const loaded = try readRegularFile(allocator, io, dir, sub_path);
    defer allocator.free(loaded.bytes);
    const before_digest = digest(loaded.bytes);
    if (!std.mem.eql(u8, &before_digest, expected_digest)) return error.StaleSource;
    const candidate = try compose(allocator, loaded.bytes, edits);
    defer allocator.free(candidate);

    return replaceCandidateInDirWithFault(allocator, io, dir, sub_path, expected_digest, candidate, fault);
}

pub fn replaceCandidateInDir(allocator: std.mem.Allocator, io: std.Io, dir: std.Io.Dir, sub_path: []const u8, expected_digest: []const u8, candidate: []const u8) !ApplyResult {
    return replaceCandidateInDirWithFault(allocator, io, dir, sub_path, expected_digest, candidate, .none);
}

fn replaceCandidateInDirWithFault(allocator: std.mem.Allocator, io: std.Io, dir: std.Io.Dir, sub_path: []const u8, expected_digest: []const u8, candidate: []const u8, fault: Fault) !ApplyResult {
    var candidate_json = std.json.parseFromSlice(std.json.Value, allocator, candidate, .{}) catch return error.InvalidResult;
    defer candidate_json.deinit();
    const loaded = try readRegularFile(allocator, io, dir, sub_path);
    defer allocator.free(loaded.bytes);
    const before_digest = digest(loaded.bytes);
    if (!std.mem.eql(u8, &before_digest, expected_digest)) return error.StaleSource;

    if (fault == .permission) return error.InjectedPermissionFailure;
    const recovery_path = try std.fmt.allocPrint(allocator, "{s}.zconfig-recovery", .{sub_path});
    defer allocator.free(recovery_path);
    if (fault == .recovery) return error.InjectedRecoveryFailure;
    try writeAtomic(io, dir, recovery_path, loaded.bytes, loaded.permissions, .none);
    if (fault == .interrupt_after_recovery) return error.Interrupted;

    // Re-read immediately before replacement so formatting-only concurrent
    // changes invalidate the operation as well.
    const fresh = try readRegularFile(allocator, io, dir, sub_path);
    defer allocator.free(fresh.bytes);
    const fresh_digest = digest(fresh.bytes);
    if (!std.mem.eql(u8, &fresh_digest, expected_digest)) return error.StaleSource;

    try writeAtomic(io, dir, sub_path, candidate, loaded.permissions, fault);
    const after_digest = digest(candidate);
    return .{ .source_digest_before = before_digest, .source_digest_after = after_digest, .recovery_available = true };
}

const LoadedFile = struct { bytes: []u8, permissions: std.Io.File.Permissions };

fn readRegularFile(allocator: std.mem.Allocator, io: std.Io, dir: std.Io.Dir, sub_path: []const u8) !LoadedFile {
    const path_stat = try dir.statFile(io, sub_path, .{ .follow_symlinks = false });
    if (path_stat.kind != .file) return error.NotRegularFile;
    var file = try dir.openFile(io, sub_path, .{});
    defer file.close(io);
    const stat = try file.stat(io);
    if (stat.kind != .file) return error.NotRegularFile;
    if (stat.size > max_source_bytes) return error.SourceTooLarge;
    var buffer: [4096]u8 = undefined;
    var reader = file.reader(io, &buffer);
    const bytes = reader.interface.allocRemaining(allocator, .limited(max_source_bytes + 1)) catch |err| switch (err) {
        error.StreamTooLong => return error.SourceTooLarge,
        else => return err,
    };
    return .{ .bytes = bytes, .permissions = stat.permissions };
}

fn writeAtomic(io: std.Io, dir: std.Io.Dir, sub_path: []const u8, bytes: []const u8, permissions: std.Io.File.Permissions, fault: Fault) !void {
    if (fault == .temp_creation) return error.InjectedTempCreationFailure;
    var atomic = try dir.createFileAtomic(io, sub_path, .{ .permissions = permissions, .replace = true });
    defer atomic.deinit(io);
    if (fault == .write) return error.InjectedWriteFailure;
    try atomic.file.writeStreamingAll(io, bytes);
    if (fault == .flush) return error.InjectedFlushFailure;
    try atomic.file.sync(io);
    if (fault == .replacement) return error.InjectedReplacementFailure;
    if (fault == .locked) return error.LockedDestination;
    try atomic.replace(io);
}

fn digest(bytes: []const u8) [71]u8 {
    var hash: [32]u8 = undefined;
    std.crypto.hash.sha2.Sha256.hash(bytes, &hash, .{});
    var result: [71]u8 = undefined;
    @memcpy(result[0..7], "sha256:");
    const hex = std.fmt.bytesToHex(hash, .lower);
    @memcpy(result[7..], &hex);
    return result;
}

pub fn compose(allocator: std.mem.Allocator, source: []const u8, edits: []const Edit) ![]u8 {
    var index = try source_index.Index.build(allocator, source);
    defer index.deinit();
    var parsed = std.json.parseFromSlice(std.json.Value, allocator, source, .{}) catch return error.InvalidSource;
    defer parsed.deinit();

    var patches: std.ArrayList(Patch) = .empty;
    defer {
        for (patches.items) |patch| allocator.free(patch.replacement);
        patches.deinit(allocator);
    }
    for (edits, 0..) |edit, order| {
        const patch = try makePatch(allocator, index, &parsed.value, edit, order);
        try patches.append(allocator, patch);
    }
    std.mem.sort(Patch, patches.items, {}, patchLessThan);
    var cursor: usize = 0;
    var output: std.ArrayList(u8) = .empty;
    errdefer output.deinit(allocator);
    for (patches.items) |patch| {
        if (patch.start < cursor) return error.OverlappingEdits;
        try output.appendSlice(allocator, source[cursor..patch.start]);
        try output.appendSlice(allocator, patch.replacement);
        cursor = patch.end;
    }
    try output.appendSlice(allocator, source[cursor..]);
    const result = try output.toOwnedSlice(allocator);
    errdefer allocator.free(result);
    var validation = std.json.parseFromSlice(std.json.Value, allocator, result, .{}) catch return error.InvalidResult;
    validation.deinit();
    return result;
}

fn makePatch(allocator: std.mem.Allocator, index: source_index.Index, root: *std.json.Value, edit: Edit, order: usize) !Patch {
    switch (edit.operation) {
        .replace => {
            const value = edit.value orelse return error.MissingReplacement;
            try validateReplacement(allocator, value);
            const envelope = try index.editEnvelope(edit.path, .replace);
            return .{ .start = envelope.start, .end = envelope.end, .replacement = try allocator.dupe(u8, value), .order = order };
        },
        .remove => {
            if (edit.value != null) return error.UnexpectedReplacement;
            const envelope = try index.editEnvelope(edit.path, .remove);
            return .{ .start = envelope.start, .end = envelope.end, .replacement = try allocator.dupe(u8, ""), .order = order };
        },
        .add => return makeAddPatch(allocator, index, root, edit, order),
    }
}

fn makeAddPatch(allocator: std.mem.Allocator, index: source_index.Index, root: *std.json.Value, edit: Edit, order: usize) !Patch {
    const value = edit.value orelse return error.MissingReplacement;
    try validateReplacement(allocator, value);
    if (edit.path.len == 0) return error.CannotAddRoot;
    if (index.find(edit.path) != null) {
        var parsed_path = try pointer.parse(allocator, edit.path);
        defer parsed_path.deinit();
        const resolved = try pointer.resolve(root, parsed_path, .existing);
        if (resolved.parent == null or resolved.parent.?.* != .array) return error.TargetAlreadyExists;
    }
    const slash = std.mem.lastIndexOfScalar(u8, edit.path, '/') orelse return error.InvalidPointer;
    const parent_path = edit.path[0..slash];
    var parsed_path = try pointer.parse(allocator, edit.path);
    defer parsed_path.deinit();
    const token = parsed_path.tokens[parsed_path.tokens.len - 1];
    const parent_node = index.find(parent_path) orelse return error.ParentNotFound;
    var replacement: std.ArrayList(u8) = .empty;
    errdefer replacement.deinit(allocator);
    switch (parent_node.container) {
        .object => {
            if (index.find(edit.path) != null) return error.TargetAlreadyExists;
            const envelope = try index.addEnvelope(parent_path);
            if (envelope.needs_comma) try replacement.append(allocator, ',');
            try appendJsonString(allocator, &replacement, token);
            try replacement.append(allocator, ':');
            try replacement.appendSlice(allocator, value);
            return .{ .start = envelope.offset, .end = envelope.offset, .replacement = try replacement.toOwnedSlice(allocator), .order = order };
        },
        .array => {
            const envelope = try index.addEnvelope(parent_path);
            if (std.mem.eql(u8, token, "-")) {
                if (envelope.needs_comma) try replacement.append(allocator, ',');
                try replacement.appendSlice(allocator, value);
                return .{ .start = envelope.offset, .end = envelope.offset, .replacement = try replacement.toOwnedSlice(allocator), .order = order };
            }
            const position = try parseIndex(token);
            var parent_pointer = try pointer.parse(allocator, parent_path);
            defer parent_pointer.deinit();
            const parent = try pointer.resolve(root, parent_pointer, .existing);
            const count = parent.value.array.items.len;
            if (position > count) return error.InvalidArrayIndex;
            if (position == count) {
                if (envelope.needs_comma) try replacement.append(allocator, ',');
                try replacement.appendSlice(allocator, value);
                return .{ .start = envelope.offset, .end = envelope.offset, .replacement = try replacement.toOwnedSlice(allocator), .order = order };
            }
            const target_path = try std.fmt.allocPrint(allocator, "{s}/{d}", .{ parent_path, position });
            defer allocator.free(target_path);
            const target = index.find(target_path) orelse return error.InvalidArrayIndex;
            try replacement.appendSlice(allocator, value);
            try replacement.append(allocator, ',');
            return .{ .start = target.member_start, .end = target.member_start, .replacement = try replacement.toOwnedSlice(allocator), .order = order };
        },
        .none => return error.ParentNotContainer,
    }
}

fn validateReplacement(allocator: std.mem.Allocator, value: []const u8) !void {
    var parsed = std.json.parseFromSlice(std.json.Value, allocator, value, .{}) catch return error.InvalidReplacement;
    parsed.deinit();
}

fn appendJsonString(allocator: std.mem.Allocator, output: *std.ArrayList(u8), value: []const u8) !void {
    try output.append(allocator, '"');
    for (value) |c| switch (c) {
        '"' => try output.appendSlice(allocator, "\\\""),
        '\\' => try output.appendSlice(allocator, "\\\\"),
        '\n' => try output.appendSlice(allocator, "\\n"),
        '\r' => try output.appendSlice(allocator, "\\r"),
        '\t' => try output.appendSlice(allocator, "\\t"),
        0...8, 11, 12, 14...0x1f => {
            var buffer: [6]u8 = undefined;
            const encoded = try std.fmt.bufPrint(&buffer, "\\u00{x:0>2}", .{c});
            try output.appendSlice(allocator, encoded);
        },
        else => try output.append(allocator, c),
    };
    try output.append(allocator, '"');
}

fn parseIndex(value: []const u8) !usize {
    if (value.len == 0 or (value.len > 1 and value[0] == '0')) return error.InvalidArrayIndex;
    for (value) |c| if (!std.ascii.isDigit(c)) return error.InvalidArrayIndex;
    return std.fmt.parseInt(usize, value, 10) catch error.InvalidArrayIndex;
}

fn patchLessThan(_: void, a: Patch, b: Patch) bool {
    if (a.start != b.start) return a.start < b.start;
    if (a.end != b.end) return a.end > b.end;
    return a.order < b.order;
}
