const std = @import("std");
const apply = @import("apply");

test "replace preserves indentation CRLF and unrelated escaped spelling" {
    const source = "{\r\n  \"title\": \"a\\u0041\",\r\n  \"count\" : 1e+02\r\n}\r\n";
    const edits = [_]apply.Edit{.{ .path = "/count", .operation = .replace, .value = "250" }};
    const result = try apply.compose(std.testing.allocator, source, &edits);
    defer std.testing.allocator.free(result);
    try std.testing.expectEqualStrings("{\r\n  \"title\": \"a\\u0041\",\r\n  \"count\" : 250\r\n}\r\n", result);
}

test "object add and remove preserve surrounding source bytes" {
    const source = "{ \"keep\" : true, \"remove\": 2 }";
    const edits = [_]apply.Edit{
        .{ .path = "/remove", .operation = .remove },
        .{ .path = "/new~1key", .operation = .add, .value = "[1,2]" },
    };
    const result = try apply.compose(std.testing.allocator, source, &edits);
    defer std.testing.allocator.free(result);
    try std.testing.expectEqualStrings("{ \"keep\" : true,\"new/key\":[1,2] }", result);
}

test "array insert append remove and multi edit ordering" {
    const source = "{\"items\":[10, 20, 30],\"tail\":false}";
    const edits = [_]apply.Edit{
        .{ .path = "/tail", .operation = .replace, .value = "true" },
        .{ .path = "/items/1", .operation = .add, .value = "15" },
        .{ .path = "/items/2", .operation = .remove },
        .{ .path = "/items/-", .operation = .add, .value = "40" },
    };
    const result = try apply.compose(std.testing.allocator, source, &edits);
    defer std.testing.allocator.free(result);
    try std.testing.expectEqualStrings("{\"items\":[10, 15,20,40],\"tail\":true}", result);
}

test "invalid replacement and overlapping edits fail closed" {
    const source = "{\"a\":1,\"b\":2}";
    const invalid = [_]apply.Edit{.{ .path = "/a", .operation = .replace, .value = "nope" }};
    try std.testing.expectError(error.InvalidReplacement, apply.compose(std.testing.allocator, source, &invalid));
    const overlap = [_]apply.Edit{
        .{ .path = "/a", .operation = .replace, .value = "3" },
        .{ .path = "/a", .operation = .remove },
    };
    try std.testing.expectError(error.OverlappingEdits, apply.compose(std.testing.allocator, source, &overlap));
}
