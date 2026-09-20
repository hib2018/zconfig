const std = @import("std");
const apply = @import("apply");

test "index approximately 10 MiB and 100000 nodes" {
    const allocator = std.testing.allocator;
    var source: std.ArrayList(u8) = .empty;
    defer source.deinit(allocator);
    try source.append(allocator, '[');
    for (0..100_000) |i| {
        if (i != 0) try source.append(allocator, ',');
        try source.append(allocator, '"');
        try source.appendNTimes(allocator, 'x', 100);
        try source.append(allocator, '"');
    }
    try source.appendSlice(allocator, "]\n");
    try std.testing.expect(source.items.len >= 9 * 1024 * 1024);
    const candidate = try apply.compose(allocator, source.items, &.{});
    defer allocator.free(candidate);
    try std.testing.expectEqualSlices(u8, source.items, candidate);
}

test "compose 1000 non-overlapping changes" {
    const allocator = std.testing.allocator;
    var source: std.ArrayList(u8) = .empty;
    defer source.deinit(allocator);
    try source.append(allocator, '{');
    for (0..1000) |i| {
        if (i != 0) try source.append(allocator, ',');
        const member = try std.fmt.allocPrint(allocator, "\"k{d}\":{d}", .{ i, i });
        defer allocator.free(member);
        try source.appendSlice(allocator, member);
    }
    try source.append(allocator, '}');
    var paths: [1000][]u8 = undefined;
    defer for (paths) |path| allocator.free(path);
    var edits: [1000]apply.Edit = undefined;
    for (&edits, 0..) |*edit, i| {
        paths[i] = try std.fmt.allocPrint(allocator, "/k{d}", .{i});
        edit.* = .{ .path = paths[i], .operation = .replace, .value = "0" };
    }
    const candidate = try apply.compose(allocator, source.items, &edits);
    defer allocator.free(candidate);
    try std.testing.expect(candidate.len != 0);
}
