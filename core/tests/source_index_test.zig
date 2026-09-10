const std = @import("std");
const source_index = @import("source_index");

test "lexical index retains exact nested value spans and spellings" {
    const source =
        \\{
        \\  "unicode\u0020key" : "a\u0041",
        \\  "numbers": [1e+02, -0.50, 3]
        \\}
    ;
    var index = try source_index.Index.build(std.testing.allocator, source);
    defer index.deinit();
    const string_node = index.find("/unicode key").?;
    try std.testing.expectEqualStrings("\"a\\u0041\"", string_node.bytes(source));
    try std.testing.expectEqualStrings("1e+02", index.find("/numbers/0").?.bytes(source));
    try std.testing.expectEqualStrings("-0.50", index.find("/numbers/1").?.bytes(source));
}

test "replace and remove envelopes are core computed" {
    const source = "{\"a\": 1, \"b\" : [ true, false ], \"c\":3}";
    var index = try source_index.Index.build(std.testing.allocator, source);
    defer index.deinit();

    const replace = try index.editEnvelope("/b/0", .replace);
    try std.testing.expectEqualStrings("true", source[replace.start..replace.end]);

    const remove_middle = try index.editEnvelope("/b/0", .remove);
    try std.testing.expectEqualStrings("true, ", source[remove_middle.start..remove_middle.end]);

    const remove_last = try index.editEnvelope("/c", .remove);
    try std.testing.expectEqualStrings(", \"c\":3", source[remove_last.start..remove_last.end]);
}

test "add envelope points before a container close and reports comma need" {
    var object = try source_index.Index.build(std.testing.allocator, "{\"a\":1}");
    defer object.deinit();
    const object_add = try object.addEnvelope("");
    try std.testing.expectEqual(@as(usize, 6), object_add.offset);
    try std.testing.expect(object_add.needs_comma);

    var empty_array = try source_index.Index.build(std.testing.allocator, "[]");
    defer empty_array.deinit();
    const array_add = try empty_array.addEnvelope("");
    try std.testing.expectEqual(@as(usize, 1), array_add.offset);
    try std.testing.expect(!array_add.needs_comma);
}

test "index rejects duplicate keys and malformed JSON" {
    try std.testing.expectError(error.DuplicateKey, source_index.Index.build(
        std.testing.allocator,
        "{\"a\":1,\"a\":2}",
    ));
    try std.testing.expectError(error.InvalidJson, source_index.Index.build(
        std.testing.allocator,
        "{\"a\":}",
    ));
}

test "missing target is reported" {
    var index = try source_index.Index.build(std.testing.allocator, "{\"a\":1}");
    defer index.deinit();
    try std.testing.expectError(error.TargetNotFound, index.editEnvelope("/missing", .replace));
}
