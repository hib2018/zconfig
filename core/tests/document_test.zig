const std = @import("std");
const document = @import("document");

test "strict document identity and duplicate rejection" {
    var parsed = try document.parse(std.testing.allocator, "{\"a\":1}");
    defer parsed.deinit();
    try std.testing.expectEqual(@as(usize, 2), parsed.node_count);
    try std.testing.expectEqual(@as(usize, 71), parsed.digest().len);
    try std.testing.expectError(error.DuplicateKey, document.parse(std.testing.allocator, "{\"a\":1,\"a\":2}"));
}
