const std = @import("std");

pub const Pointer = struct {
    allocator: std.mem.Allocator,
    tokens: []const []const u8,

    pub fn deinit(self: *Pointer) void {
        for (self.tokens) |token| self.allocator.free(token);
        self.allocator.free(self.tokens);
        self.* = undefined;
    }
};

pub fn parse(allocator: std.mem.Allocator, text: []const u8) !Pointer {
    if (text.len != 0 and text[0] != '/') return error.InvalidPointer;
    var tokens: std.ArrayList([]const u8) = .empty;
    errdefer {
        for (tokens.items) |token| allocator.free(token);
        tokens.deinit(allocator);
    }
    if (text.len != 0) {
        var iterator = std.mem.splitScalar(u8, text[1..], '/');
        while (iterator.next()) |encoded| {
            var decoded: std.ArrayList(u8) = .empty;
            errdefer decoded.deinit(allocator);
            var i: usize = 0;
            while (i < encoded.len) : (i += 1) {
                if (encoded[i] != '~') {
                    try decoded.append(allocator, encoded[i]);
                    continue;
                }
                if (i + 1 >= encoded.len) return error.InvalidEscape;
                i += 1;
                try decoded.append(allocator, switch (encoded[i]) {
                    '0' => '~',
                    '1' => '/',
                    else => return error.InvalidEscape,
                });
            }
            try tokens.append(allocator, try decoded.toOwnedSlice(allocator));
        }
    }
    return .{ .allocator = allocator, .tokens = try tokens.toOwnedSlice(allocator) };
}

pub const ResolveMode = enum { existing, allow_append };
pub const Resolved = struct {
    value: *std.json.Value,
    parent: ?*std.json.Value,
    index: ?usize = null,
    key: ?[]const u8 = null,
    is_append: bool = false,
};

pub fn resolve(root: *std.json.Value, parsed: Pointer, mode: ResolveMode) !Resolved {
    if (parsed.tokens.len == 0) return .{ .value = root, .parent = null };
    var current = root;
    for (parsed.tokens, 0..) |token, token_index| {
        const last = token_index + 1 == parsed.tokens.len;
        switch (current.*) {
            .object => |*object| {
                const child = object.getPtr(token) orelse return error.TargetNotFound;
                if (last) return .{ .value = child, .parent = current, .key = token };
                current = child;
            },
            .array => |*array| {
                if (std.mem.eql(u8, token, "-")) {
                    if (last and mode == .allow_append)
                        return .{ .value = current, .parent = current, .index = array.items.len, .is_append = true };
                    return error.TargetNotFound;
                }
                const index = parseIndex(token) catch return error.TargetNotFound;
                if (index >= array.items.len) return error.TargetNotFound;
                if (last) return .{ .value = &array.items[index], .parent = current, .index = index };
                current = &array.items[index];
            },
            else => return error.TargetNotFound,
        }
    }
    unreachable;
}

fn parseIndex(token: []const u8) !usize {
    if (token.len == 0 or (token.len > 1 and token[0] == '0')) return error.InvalidIndex;
    for (token) |c| if (!std.ascii.isDigit(c)) return error.InvalidIndex;
    return std.fmt.parseInt(usize, token, 10) catch error.InvalidIndex;
}
