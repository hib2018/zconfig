const std = @import("std");

pub const protocol = @import("protocol.zig");
pub const document = @import("document.zig");
pub const pointer = @import("pointer.zig");
pub const proposal = @import("proposal.zig");
pub const redact = @import("redact.zig");
pub const schema = @import("schema.zig");
pub const source_index = @import("source_index.zig");
pub const apply = @import("apply.zig");
pub const protocol_version = protocol.protocol_version;
pub const max_message_bytes = protocol.max_message_bytes;

test "foundation constants match the protocol contract" {
    try std.testing.expectEqualStrings("1.0", protocol_version);
    try std.testing.expectEqual(@as(usize, 16 * 1024 * 1024), max_message_bytes);
}
