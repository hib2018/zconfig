const std = @import("std");
const protocol = @import("protocol.zig");
const pointer = @import("pointer.zig");

pub const ParsedProposal = std.json.Parsed(protocol.Proposal);

pub fn parse(allocator: std.mem.Allocator, bytes: []const u8) !ParsedProposal {
    var shape = std.json.parseFromSlice(std.json.Value, allocator, bytes, .{}) catch |err| return switch (err) {
        error.DuplicateField => error.DuplicateField,
        else => error.InvalidProposal,
    };
    defer shape.deinit();
    try validateShape(shape.value);
    var parsed = protocol.parseStrict(protocol.Proposal, allocator, bytes) catch |err| return err;
    errdefer parsed.deinit();
    try validate(parsed.value);
    return parsed;
}

pub fn validateAgainstSource(allocator: std.mem.Allocator, value: protocol.Proposal, root: *std.json.Value) !void {
    try validate(value);
    for (value.items) |item| {
        var parsed_pointer = try pointer.parse(allocator, item.path);
        defer parsed_pointer.deinit();
        switch (item.operation) {
            .replace, .remove => {
                const resolved = pointer.resolve(root, parsed_pointer, .existing) catch return error.TargetNotFound;
                if (!valueEqual(resolved.value.*, item.expected_old.?)) return error.ExpectedOldMismatch;
            },
            .add => {
                if (pointer.resolve(root, parsed_pointer, .allow_append)) |_| {
                    if (!std.mem.endsWith(u8, item.path, "/-")) return error.TargetAlreadyExists;
                } else |_| try validateAddParent(allocator, root, item.path);
            },
        }
    }
}

pub fn validateRevision(active: protocol.Proposal, candidate: protocol.Proposal, allowed_ids: []const []const u8) !void {
    try validate(active);
    try validate(candidate);
    if (!std.mem.eql(u8, active.proposal_id, candidate.proposal_id) or
        !std.mem.eql(u8, active.source_digest, candidate.source_digest) or
        candidate.revision != active.revision + 1 or active.items.len != candidate.items.len)
        return error.StaleRevisionBase;
    for (active.items, candidate.items) |before, after| {
        if (!std.mem.eql(u8, before.change_id, after.change_id) or
            !std.mem.eql(u8, before.path, after.path) or before.operation != after.operation or
            !optionalValueEqual(before.expected_old, after.expected_old))
            return error.ScopeViolation;
        if (!contains(allowed_ids, before.change_id) and
            (!optionalValueEqual(before.proposed_value, after.proposed_value) or
                !std.mem.eql(u8, before.explanation, after.explanation)))
            return error.ScopeViolation;
    }
}

fn contains(values: []const []const u8, target: []const u8) bool {
    for (values) |value| if (std.mem.eql(u8, value, target)) return true;
    return false;
}

fn optionalValueEqual(a: ?std.json.Value, b: ?std.json.Value) bool {
    if (a == null or b == null) return a == null and b == null;
    return valueEqual(a.?, b.?);
}

fn validateAddParent(allocator: std.mem.Allocator, root: *std.json.Value, path: []const u8) !void {
    const slash = std.mem.lastIndexOfScalar(u8, path, '/') orelse return error.InvalidPointer;
    const parent_text = path[0..slash];
    var parent = try pointer.parse(allocator, parent_text);
    defer parent.deinit();
    const resolved = pointer.resolve(root, parent, .existing) catch return error.ParentNotFound;
    if (resolved.value.* != .object and resolved.value.* != .array) return error.ParentNotContainer;
}

fn valueEqual(a: std.json.Value, b: std.json.Value) bool {
    if (@intFromEnum(a) != @intFromEnum(b)) return numeric(a) != null and numeric(b) != null and numeric(a).? == numeric(b).?;
    return switch (a) {
        .null => true,
        .bool => |v| v == b.bool,
        .integer => |v| v == b.integer,
        .float => |v| v == b.float,
        .number_string => |v| std.mem.eql(u8, v, b.number_string),
        .string => |v| std.mem.eql(u8, v, b.string),
        .array => |v| blk: {
            if (v.items.len != b.array.items.len) break :blk false;
            for (v.items, b.array.items) |x, y| if (!valueEqual(x, y)) break :blk false;
            break :blk true;
        },
        .object => |v| blk: {
            if (v.count() != b.object.count()) break :blk false;
            var iterator = v.iterator();
            while (iterator.next()) |entry| {
                const other = b.object.get(entry.key_ptr.*) orelse break :blk false;
                if (!valueEqual(entry.value_ptr.*, other)) break :blk false;
            }
            break :blk true;
        },
    };
}

fn numeric(value: std.json.Value) ?f64 {
    return switch (value) {
        .integer => |v| @floatFromInt(v),
        .float => |v| v,
        .number_string => |v| std.fmt.parseFloat(f64, v) catch null,
        else => null,
    };
}

pub fn validate(value: protocol.Proposal) !void {
    if (value.proposal_id.len == 0 or value.proposal_id.len > 128) return error.InvalidProposalId;
    if (value.revision == 0) return error.InvalidRevision;
    if (!validDigest(value.source_digest)) return error.InvalidDigest;
    if (value.created_at.len < 20 or std.mem.indexOfScalar(u8, value.created_at, 'T') == null) return error.InvalidTimestamp;
    if (value.producer) |producer| if (producer.len > 256) return error.InvalidProducer;
    if (value.items.len == 0 or value.items.len > 1000) return error.InvalidItemCount;
    for (value.items, 0..) |item, i| {
        if (item.change_id.len == 0 or item.change_id.len > 128) return error.InvalidChangeId;
        if (item.explanation.len > 8192) return error.InvalidExplanation;
        if (!validPointer(item.path)) return error.InvalidPointer;
        for (value.items[0..i]) |previous| {
            if (std.mem.eql(u8, item.change_id, previous.change_id)) return error.DuplicateChangeId;
            if (pathsOverlap(item.path, previous.path)) return error.OverlappingTargets;
        }
    }
}

fn validateShape(root: std.json.Value) !void {
    const object = switch (root) {
        .object => |v| v,
        else => return error.InvalidProposal,
    };
    const items_value = object.get("items") orelse return error.InvalidProposal;
    const items = switch (items_value) {
        .array => |v| v,
        else => return error.InvalidProposal,
    };
    for (items.items) |item_value| {
        const item = switch (item_value) {
            .object => |v| v,
            else => return error.InvalidProposal,
        };
        const operation_value = item.get("operation") orelse return error.InvalidProposal;
        const operation = switch (operation_value) {
            .string => |v| v,
            else => return error.InvalidProposal,
        };
        const has_old = item.contains("expected_old");
        const has_new = item.contains("proposed_value");
        if (std.mem.eql(u8, operation, "add") and (!has_new or has_old)) return error.InvalidOperationShape;
        if (std.mem.eql(u8, operation, "replace") and (!has_new or !has_old)) return error.InvalidOperationShape;
        if (std.mem.eql(u8, operation, "remove") and (!has_old or has_new)) return error.InvalidOperationShape;
    }
}

pub fn validDigest(value: []const u8) bool {
    if (value.len != 71 or !std.mem.startsWith(u8, value, "sha256:")) return false;
    for (value[7..]) |c| if (!std.ascii.isDigit(c) and !(c >= 'a' and c <= 'f')) return false;
    return true;
}

pub fn validPointer(path: []const u8) bool {
    if (path.len == 0) return true;
    if (path[0] != '/') return false;
    var i: usize = 0;
    while (i < path.len) : (i += 1) {
        if (path[i] == '~') {
            if (i + 1 >= path.len or (path[i + 1] != '0' and path[i + 1] != '1')) return false;
            i += 1;
        }
    }
    return true;
}

fn pathsOverlap(a: []const u8, b: []const u8) bool {
    if (std.mem.eql(u8, a, b)) return true;
    if (a.len < b.len and std.mem.startsWith(u8, b, a) and b[a.len] == '/') return true;
    if (b.len < a.len and std.mem.startsWith(u8, a, b) and a[b.len] == '/') return true;
    return false;
}
