const std = @import("std");

pub const Container = enum { none, object, array };

pub const Node = struct {
    pointer: []const u8,
    start: usize,
    end: usize,
    member_start: usize,
    close_offset: ?usize = null,
    container: Container = .none,
    parent: ?usize = null,
    ordinal: usize = 0,

    pub fn bytes(self: Node, source: []const u8) []const u8 {
        return source[self.start..self.end];
    }
};

pub const EditKind = enum { replace, remove };
pub const Envelope = struct { start: usize, end: usize };
pub const AddEnvelope = struct { offset: usize, needs_comma: bool, container: Container };

pub const Index = struct {
    allocator: std.mem.Allocator,
    source: []const u8,
    nodes: []Node,

    pub fn build(allocator: std.mem.Allocator, source: []const u8) !Index {
        var validation = std.json.parseFromSlice(std.json.Value, allocator, source, .{}) catch |err| return switch (err) {
            error.DuplicateField => error.DuplicateKey,
            else => error.InvalidJson,
        };
        defer validation.deinit();

        var parser = Parser{ .allocator = allocator, .source = source };
        errdefer parser.deinit();
        _ = try parser.value("", null, 0, parser.skipWhitespace());
        parser.pos = parser.skipWhitespace();
        if (parser.pos != source.len) return error.InvalidJson;
        return .{ .allocator = allocator, .source = source, .nodes = try parser.nodes.toOwnedSlice(allocator) };
    }

    pub fn deinit(self: *Index) void {
        for (self.nodes) |node| self.allocator.free(node.pointer);
        self.allocator.free(self.nodes);
        self.* = undefined;
    }

    pub fn find(self: Index, path: []const u8) ?Node {
        for (self.nodes) |node| if (std.mem.eql(u8, node.pointer, path)) return node;
        return null;
    }

    fn findIndex(self: Index, path: []const u8) ?usize {
        for (self.nodes, 0..) |node, i| if (std.mem.eql(u8, node.pointer, path)) return i;
        return null;
    }

    pub fn editEnvelope(self: Index, path: []const u8, kind: EditKind) !Envelope {
        const index = self.findIndex(path) orelse return error.TargetNotFound;
        const node = self.nodes[index];
        if (kind == .replace) return .{ .start = node.start, .end = node.end };
        const parent = node.parent orelse return error.CannotRemoveRoot;
        for (self.nodes[index + 1 ..]) |candidate| {
            if (candidate.parent == parent and candidate.ordinal == node.ordinal + 1)
                return .{ .start = node.member_start, .end = candidate.member_start };
        }
        var i = index;
        while (i > 0) {
            i -= 1;
            const candidate = self.nodes[i];
            if (candidate.parent == parent and candidate.ordinal + 1 == node.ordinal)
                return .{ .start = candidate.end, .end = node.end };
        }
        return .{ .start = node.member_start, .end = node.end };
    }

    pub fn addEnvelope(self: Index, container_path: []const u8) !AddEnvelope {
        const index = self.findIndex(container_path) orelse return error.TargetNotFound;
        const node = self.nodes[index];
        if (node.container == .none) return error.NotContainer;
        var children: usize = 0;
        for (self.nodes) |candidate| if (candidate.parent == index) {
            children += 1;
        };
        return .{ .offset = node.close_offset.?, .needs_comma = children != 0, .container = node.container };
    }
};

const Parser = struct {
    allocator: std.mem.Allocator,
    source: []const u8,
    pos: usize = 0,
    nodes: std.ArrayList(Node) = .empty,

    fn deinit(self: *Parser) void {
        for (self.nodes.items) |node| self.allocator.free(node.pointer);
        self.nodes.deinit(self.allocator);
    }

    fn skipWhitespace(self: *Parser) usize {
        while (self.pos < self.source.len and std.ascii.isWhitespace(self.source[self.pos])) self.pos += 1;
        return self.pos;
    }

    fn value(self: *Parser, path: []const u8, parent: ?usize, ordinal: usize, member_start: usize) anyerror!usize {
        _ = self.skipWhitespace();
        const start = self.pos;
        const node_index = self.nodes.items.len;
        try self.nodes.append(self.allocator, .{
            .pointer = try self.allocator.dupe(u8, path),
            .start = start,
            .end = undefined,
            .member_start = member_start,
            .parent = parent,
            .ordinal = ordinal,
        });
        if (self.pos >= self.source.len) return error.InvalidJson;
        switch (self.source[self.pos]) {
            '{' => try self.object(node_index, path),
            '[' => try self.array(node_index, path),
            '"' => _ = try self.stringEnd(),
            else => while (self.pos < self.source.len and
                !std.ascii.isWhitespace(self.source[self.pos]) and
                self.source[self.pos] != ',' and self.source[self.pos] != ']' and self.source[self.pos] != '}')
            {
                self.pos += 1;
            },
        }
        self.nodes.items[node_index].end = self.pos;
        return node_index;
    }

    fn object(self: *Parser, node_index: usize, path: []const u8) anyerror!void {
        self.nodes.items[node_index].container = .object;
        self.pos += 1;
        _ = self.skipWhitespace();
        var keys: std.StringHashMap(void) = .init(self.allocator);
        defer {
            var iterator = keys.keyIterator();
            while (iterator.next()) |key| self.allocator.free(key.*);
            keys.deinit();
        }
        var ordinal: usize = 0;
        while (self.pos < self.source.len and self.source[self.pos] != '}') : (ordinal += 1) {
            const member_start = self.pos;
            const key_start = self.pos;
            const key_end = try self.stringEnd();
            var parsed_key = std.json.parseFromSlice([]const u8, self.allocator, self.source[key_start..key_end], .{}) catch
                return error.InvalidJson;
            defer parsed_key.deinit();
            const key = try self.allocator.dupe(u8, parsed_key.value);
            if (keys.contains(key)) {
                self.allocator.free(key);
                return error.DuplicateKey;
            }
            try keys.put(key, {});
            _ = self.skipWhitespace();
            if (self.pos >= self.source.len or self.source[self.pos] != ':') return error.InvalidJson;
            self.pos += 1;
            const child_path = try appendToken(self.allocator, path, parsed_key.value);
            defer self.allocator.free(child_path);
            _ = try self.value(child_path, node_index, ordinal, member_start);
            _ = self.skipWhitespace();
            if (self.pos < self.source.len and self.source[self.pos] == ',') {
                self.pos += 1;
                _ = self.skipWhitespace();
            } else break;
        }
        if (self.pos >= self.source.len or self.source[self.pos] != '}') return error.InvalidJson;
        self.nodes.items[node_index].close_offset = self.pos;
        self.pos += 1;
    }

    fn array(self: *Parser, node_index: usize, path: []const u8) anyerror!void {
        self.nodes.items[node_index].container = .array;
        self.pos += 1;
        _ = self.skipWhitespace();
        var ordinal: usize = 0;
        while (self.pos < self.source.len and self.source[self.pos] != ']') : (ordinal += 1) {
            const member_start = self.pos;
            const child_path = try std.fmt.allocPrint(self.allocator, "{s}/{d}", .{ path, ordinal });
            defer self.allocator.free(child_path);
            _ = try self.value(child_path, node_index, ordinal, member_start);
            _ = self.skipWhitespace();
            if (self.pos < self.source.len and self.source[self.pos] == ',') {
                self.pos += 1;
                _ = self.skipWhitespace();
            } else break;
        }
        if (self.pos >= self.source.len or self.source[self.pos] != ']') return error.InvalidJson;
        self.nodes.items[node_index].close_offset = self.pos;
        self.pos += 1;
    }

    fn stringEnd(self: *Parser) !usize {
        if (self.pos >= self.source.len or self.source[self.pos] != '"') return error.InvalidJson;
        self.pos += 1;
        while (self.pos < self.source.len) {
            if (self.source[self.pos] == '"') {
                self.pos += 1;
                return self.pos;
            }
            if (self.source[self.pos] == '\\') self.pos += 1;
            self.pos += 1;
        }
        return error.InvalidJson;
    }
};

fn appendToken(allocator: std.mem.Allocator, path: []const u8, token: []const u8) ![]u8 {
    var output: std.ArrayList(u8) = .empty;
    defer output.deinit(allocator);
    try output.appendSlice(allocator, path);
    try output.append(allocator, '/');
    for (token) |c| switch (c) {
        '~' => try output.appendSlice(allocator, "~0"),
        '/' => try output.appendSlice(allocator, "~1"),
        else => try output.append(allocator, c),
    };
    return output.toOwnedSlice(allocator);
}
