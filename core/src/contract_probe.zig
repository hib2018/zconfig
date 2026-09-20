const std = @import("std");
const core = @import("zconfig_core");

pub fn main(init: std.process.Init) !void {
    const allocator = init.arena.allocator();
    const args = try init.minimal.args.toSlice(allocator);
    if (args.len < 2 or args.len > 3) return error.InvalidArguments;
    var buffer: [4096]u8 = undefined;
    var stdin = std.Io.File.stdin().reader(init.io, &buffer);
    const input = try stdin.interface.allocRemaining(allocator, .limited(core.max_message_bytes + 1));
    const expected = if (args.len == 3) args[2] else null;
    const valid = if (core.contract.validate(args[1], allocator, input, expected)) |_| true else |_| false;
    var output_buffer: [32]u8 = undefined;
    var stdout = std.Io.File.stdout().writer(init.io, &output_buffer);
    try stdout.interface.writeAll(if (valid) "valid\n" else "invalid\n");
    try stdout.flush();
}
