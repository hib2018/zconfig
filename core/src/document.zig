const std = @import("std");

pub const max_source_bytes: usize = 10 * 1024 * 1024;
pub const max_nodes: usize = 100_000;

pub const Document = struct {
    parsed: std.json.Parsed(std.json.Value),
    digest_buf: [71]u8,
    byte_length: usize,
    node_count: usize,

    pub fn deinit(self: *Document) void {
        self.parsed.deinit();
    }

    pub fn digest(self: *const Document) []const u8 {
        return &self.digest_buf;
    }
};

pub fn parse(allocator: std.mem.Allocator, bytes: []const u8) !Document {
    if (bytes.len > max_source_bytes) return error.SourceTooLarge;
    var parsed = std.json.parseFromSlice(std.json.Value, allocator, bytes, .{}) catch |err| return switch (err) {
        error.DuplicateField => error.DuplicateKey,
        else => error.InvalidJson,
    };
    errdefer parsed.deinit();
    var count: usize = 0;
    try countValue(parsed.value, &count);
    var hash: [32]u8 = undefined;
    std.crypto.hash.sha2.Sha256.hash(bytes, &hash, .{});
    var digest_buf: [71]u8 = undefined;
    @memcpy(digest_buf[0..7], "sha256:");
    const hex = std.fmt.bytesToHex(hash, .lower);
    @memcpy(digest_buf[7..], &hex);
    return .{ .parsed = parsed, .digest_buf = digest_buf, .byte_length = bytes.len, .node_count = count };
}

pub const Loaded = struct {
    bytes: []u8,
    document: Document,

    pub fn deinit(self: *Loaded, allocator: std.mem.Allocator) void {
        self.document.deinit();
        allocator.free(self.bytes);
        self.* = undefined;
    }
};

pub fn load(allocator: std.mem.Allocator, io: std.Io, path: []const u8) !Loaded {
    var file = try std.Io.Dir.cwd().openFile(io, path, .{});
    defer file.close(io);
    const stat = try file.stat(io);
    if (stat.kind != .file) return error.NotRegularFile;
    if (stat.size > max_source_bytes) return error.SourceTooLarge;
    var buffer: [4096]u8 = undefined;
    var reader = file.reader(io, &buffer);
    const bytes = reader.interface.allocRemaining(allocator, .limited(max_source_bytes + 1)) catch |err| switch (err) {
        error.StreamTooLong => return error.SourceTooLarge,
        else => return err,
    };
    errdefer allocator.free(bytes);
    return .{ .bytes = bytes, .document = try parse(allocator, bytes) };
}

fn countValue(value: std.json.Value, count: *usize) !void {
    count.* += 1;
    if (count.* > max_nodes) return error.TooManyNodes;
    switch (value) {
        .array => |items| for (items.items) |item| try countValue(item, count),
        .object => |object| {
            var iterator = object.iterator();
            while (iterator.next()) |entry| try countValue(entry.value_ptr.*, count);
        },
        else => {},
    }
}

pub fn digestMatches(actual: []const u8, expected: []const u8) bool {
    return std.mem.eql(u8, actual, expected);
}
