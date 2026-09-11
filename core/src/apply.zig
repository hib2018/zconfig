const std = @import("std");
const source_index = @import("source_index.zig");
const pointer = @import("pointer.zig");

pub const Operation = enum { add, replace, remove };
pub const Edit = struct { path: []const u8, operation: Operation, value: ?[]const u8 = null };

const Patch = struct { start: usize, end: usize, replacement: []u8, order: usize };

pub fn compose(allocator: std.mem.Allocator, source: []const u8, edits: []const Edit) ![]u8 {
    var index = try source_index.Index.build(allocator, source);
    defer index.deinit();
    var parsed = std.json.parseFromSlice(std.json.Value, allocator, source, .{}) catch return error.InvalidSource;
    defer parsed.deinit();

    var patches: std.ArrayList(Patch) = .empty;
    defer {
        for (patches.items) |patch| allocator.free(patch.replacement);
        patches.deinit(allocator);
    }
    for (edits, 0..) |edit, order| {
        const patch = try makePatch(allocator, index, &parsed.value, edit, order);
        try patches.append(allocator, patch);
    }
    std.mem.sort(Patch, patches.items, {}, patchLessThan);
    var cursor: usize = 0;
    var output: std.ArrayList(u8) = .empty;
    errdefer output.deinit(allocator);
    for (patches.items) |patch| {
        if (patch.start < cursor) return error.OverlappingEdits;
        try output.appendSlice(allocator, source[cursor..patch.start]);
        try output.appendSlice(allocator, patch.replacement);
        cursor = patch.end;
    }
    try output.appendSlice(allocator, source[cursor..]);
    const result = try output.toOwnedSlice(allocator);
    errdefer allocator.free(result);
    var validation = std.json.parseFromSlice(std.json.Value, allocator, result, .{}) catch return error.InvalidResult;
    validation.deinit();
    return result;
}

fn makePatch(allocator: std.mem.Allocator, index: source_index.Index, root: *std.json.Value, edit: Edit, order: usize) !Patch {
    switch (edit.operation) {
        .replace => {
            const value = edit.value orelse return error.MissingReplacement;
            try validateReplacement(allocator, value);
            const envelope = try index.editEnvelope(edit.path, .replace);
            return .{ .start = envelope.start, .end = envelope.end, .replacement = try allocator.dupe(u8, value), .order = order };
        },
        .remove => {
            if (edit.value != null) return error.UnexpectedReplacement;
            const envelope = try index.editEnvelope(edit.path, .remove);
            return .{ .start = envelope.start, .end = envelope.end, .replacement = try allocator.dupe(u8, ""), .order = order };
        },
        .add => return makeAddPatch(allocator, index, root, edit, order),
    }
}

fn makeAddPatch(allocator: std.mem.Allocator, index: source_index.Index, root: *std.json.Value, edit: Edit, order: usize) !Patch {
    const value = edit.value orelse return error.MissingReplacement;
    try validateReplacement(allocator, value);
    if (edit.path.len == 0) return error.CannotAddRoot;
    if (index.find(edit.path) != null) {
        var parsed_path = try pointer.parse(allocator, edit.path);
        defer parsed_path.deinit();
        const resolved = try pointer.resolve(root, parsed_path, .existing);
        if (resolved.parent == null or resolved.parent.?.* != .array) return error.TargetAlreadyExists;
    }
    const slash = std.mem.lastIndexOfScalar(u8, edit.path, '/') orelse return error.InvalidPointer;
    const parent_path = edit.path[0..slash];
    var parsed_path = try pointer.parse(allocator, edit.path);
    defer parsed_path.deinit();
    const token = parsed_path.tokens[parsed_path.tokens.len - 1];
    const parent_node = index.find(parent_path) orelse return error.ParentNotFound;
    var replacement: std.ArrayList(u8) = .empty;
    errdefer replacement.deinit(allocator);
    switch (parent_node.container) {
        .object => {
            if (index.find(edit.path) != null) return error.TargetAlreadyExists;
            const envelope = try index.addEnvelope(parent_path);
            if (envelope.needs_comma) try replacement.append(allocator, ',');
            try appendJsonString(allocator, &replacement, token);
            try replacement.append(allocator, ':');
            try replacement.appendSlice(allocator, value);
            return .{ .start = envelope.offset, .end = envelope.offset, .replacement = try replacement.toOwnedSlice(allocator), .order = order };
        },
        .array => {
            const envelope = try index.addEnvelope(parent_path);
            if (std.mem.eql(u8, token, "-")) {
                if (envelope.needs_comma) try replacement.append(allocator, ',');
                try replacement.appendSlice(allocator, value);
                return .{ .start = envelope.offset, .end = envelope.offset, .replacement = try replacement.toOwnedSlice(allocator), .order = order };
            }
            const position = try parseIndex(token);
            var parent_pointer = try pointer.parse(allocator, parent_path);
            defer parent_pointer.deinit();
            const parent = try pointer.resolve(root, parent_pointer, .existing);
            const count = parent.value.array.items.len;
            if (position > count) return error.InvalidArrayIndex;
            if (position == count) {
                if (envelope.needs_comma) try replacement.append(allocator, ',');
                try replacement.appendSlice(allocator, value);
                return .{ .start = envelope.offset, .end = envelope.offset, .replacement = try replacement.toOwnedSlice(allocator), .order = order };
            }
            const target_path = try std.fmt.allocPrint(allocator, "{s}/{d}", .{ parent_path, position });
            defer allocator.free(target_path);
            const target = index.find(target_path) orelse return error.InvalidArrayIndex;
            try replacement.appendSlice(allocator, value);
            try replacement.append(allocator, ',');
            return .{ .start = target.member_start, .end = target.member_start, .replacement = try replacement.toOwnedSlice(allocator), .order = order };
        },
        .none => return error.ParentNotContainer,
    }
}

fn validateReplacement(allocator: std.mem.Allocator, value: []const u8) !void {
    var parsed = std.json.parseFromSlice(std.json.Value, allocator, value, .{}) catch return error.InvalidReplacement;
    parsed.deinit();
}

fn appendJsonString(allocator: std.mem.Allocator, output: *std.ArrayList(u8), value: []const u8) !void {
    try output.append(allocator, '"');
    for (value) |c| switch (c) {
        '"' => try output.appendSlice(allocator, "\\\""),
        '\\' => try output.appendSlice(allocator, "\\\\"),
        '\n' => try output.appendSlice(allocator, "\\n"),
        '\r' => try output.appendSlice(allocator, "\\r"),
        '\t' => try output.appendSlice(allocator, "\\t"),
        0...8, 11, 12, 14...0x1f => {
            var buffer: [6]u8 = undefined;
            const encoded = try std.fmt.bufPrint(&buffer, "\\u00{x:0>2}", .{c});
            try output.appendSlice(allocator, encoded);
        },
        else => try output.append(allocator, c),
    };
    try output.append(allocator, '"');
}

fn parseIndex(value: []const u8) !usize {
    if (value.len == 0 or (value.len > 1 and value[0] == '0')) return error.InvalidArrayIndex;
    for (value) |c| if (!std.ascii.isDigit(c)) return error.InvalidArrayIndex;
    return std.fmt.parseInt(usize, value, 10) catch error.InvalidArrayIndex;
}

fn patchLessThan(_: void, a: Patch, b: Patch) bool {
    if (a.start != b.start) return a.start < b.start;
    if (a.end != b.end) return a.end > b.end;
    return a.order < b.order;
}
