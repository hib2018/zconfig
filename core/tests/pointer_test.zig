const std = @import("std");
const pointer = @import("pointer");

test "RFC 6901 root and escaped reference tokens" {
    var root = try pointer.parse(std.testing.allocator, "");
    defer root.deinit();
    try std.testing.expectEqual(@as(usize, 0), root.tokens.len);

    var parsed = try pointer.parse(std.testing.allocator, "/a~1b/m~0n// ");
    defer parsed.deinit();
    try std.testing.expectEqualStrings("a/b", parsed.tokens[0]);
    try std.testing.expectEqualStrings("m~n", parsed.tokens[1]);
    try std.testing.expectEqualStrings("", parsed.tokens[2]);
    try std.testing.expectEqualStrings(" ", parsed.tokens[3]);
}

test "pointer resolves object keys arrays and append position" {
    var document = try std.json.parseFromSlice(std.json.Value, std.testing.allocator,
        \\{"a/b":{"items":[10,20]}}
    , .{});
    defer document.deinit();

    var item = try pointer.parse(std.testing.allocator, "/a~1b/items/1");
    defer item.deinit();
    const resolved = try pointer.resolve(&document.value, item, .existing);
    try std.testing.expectEqual(@as(i64, 20), resolved.value.integer);

    var append = try pointer.parse(std.testing.allocator, "/a~1b/items/-");
    defer append.deinit();
    const position = try pointer.resolve(&document.value, append, .allow_append);
    try std.testing.expect(position.is_append);
    try std.testing.expectEqual(@as(usize, 2), position.index.?);
}

test "pointer rejects malformed escapes indices and missing targets" {
    try std.testing.expectError(error.InvalidPointer, pointer.parse(std.testing.allocator, "key"));
    try std.testing.expectError(error.InvalidEscape, pointer.parse(std.testing.allocator, "/a~2b"));

    var document = try std.json.parseFromSlice(std.json.Value, std.testing.allocator,
        \\{"items":[1]}
    , .{});
    defer document.deinit();
    for ([_][]const u8{ "/items/01", "/items/-1", "/items/2", "/missing" }) |text| {
        var parsed = try pointer.parse(std.testing.allocator, text);
        defer parsed.deinit();
        try std.testing.expectError(error.TargetNotFound, pointer.resolve(&document.value, parsed, .existing));
    }
}
