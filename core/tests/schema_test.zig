const std = @import("std");
const schema = @import("schema");

test "common schema subset passes and fails constraints" {
    var value = try std.json.parseFromSlice(std.json.Value, std.testing.allocator, "5", .{});
    defer value.deinit();
    var valid = try schema.parse(std.testing.allocator, "{\"type\":\"integer\",\"minimum\":1,\"maximum\":10}");
    defer valid.deinit();
    try std.testing.expectEqual(schema.Status.passed, schema.validate(value.value, valid.parsed.value).status);
    var invalid = try schema.parse(std.testing.allocator, "{\"minimum\":6}");
    defer invalid.deinit();
    try std.testing.expectEqual(schema.Status.failed, schema.validate(value.value, invalid.parsed.value).status);
}

test "unsupported applicable keyword is unverified and annotation is recognized" {
    var value = try std.json.parseFromSlice(std.json.Value, std.testing.allocator, "\"x\"", .{});
    defer value.deinit();
    var unsupported = try schema.parse(std.testing.allocator, "{\"format\":\"email\"}");
    defer unsupported.deinit();
    try std.testing.expectEqual(schema.Status.unverified, schema.validate(value.value, unsupported.parsed.value).status);
    var sensitive = try schema.parse(std.testing.allocator, "{\"x-zconfig-sensitive\":true}");
    defer sensitive.deinit();
    try std.testing.expect(schema.isSensitive(sensitive.parsed.value));
}

test "sensitivity lookup follows properties items and sensitive ancestors" {
    var parsed = try schema.parse(std.testing.allocator,
        \\{"type":"object","properties":{"plain":{"type":"string"},"secrets":{"type":"array","x-zconfig-sensitive":true,"items":{"type":"object","properties":{"value":{"type":"string"}}}}}}
    );
    defer parsed.deinit();
    try std.testing.expect(!(try schema.isSensitiveAtPointer(std.testing.allocator, parsed.parsed.value, "/plain")));
    try std.testing.expect(try schema.isSensitiveAtPointer(std.testing.allocator, parsed.parsed.value, "/secrets/0/value"));
}
