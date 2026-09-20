const std = @import("std");
pub const protocol = @import("protocol.zig");
const proposal = @import("proposal.zig");
const apply = @import("apply.zig");
const source_index = @import("source_index.zig");
const redact = @import("redact.zig");
const schema = @import("schema.zig");

pub const Assembly = struct {
    allocator: std.mem.Allocator,
    candidate: []u8,
    approved_ids: [][]const u8,
    final_digest: [71]u8,

    pub fn deinit(self: *Assembly) void {
        for (self.approved_ids) |id| self.allocator.free(id);
        self.allocator.free(self.approved_ids);
        self.allocator.free(self.candidate);
        self.* = undefined;
    }
};

pub const Capability = struct {
    nonce: []const u8,
    source_digest: []const u8,
    final_change_digest: []const u8,
    issued_at_ms: i64,
    expires_at_ms: i64,
    consumed: bool = false,

    pub fn authorize(self: *Capability, now_ms: i64, nonce: []const u8, source_digest: []const u8, final_digest: []const u8) !void {
        if (self.consumed) return error.ConfirmationConsumed;
        if (now_ms < self.issued_at_ms or now_ms > self.expires_at_ms) return error.ConfirmationExpired;
        if (!std.mem.eql(u8, self.nonce, nonce)) return error.ConfirmationMismatch;
        if (!std.mem.eql(u8, self.source_digest, source_digest) or !std.mem.eql(u8, self.final_change_digest, final_digest)) return error.ConfirmationDigestMismatch;
        self.consumed = true;
    }
};

const StoredCapability = struct {
    nonce: []const u8,
    source_digest: []const u8,
    final_change_digest: []const u8,
    issued_at_ms: i64,
    expires_at_ms: i64,
};

pub fn issueCapability(allocator: std.mem.Allocator, io: std.Io, source_digest: []const u8, final_digest: []const u8, now_ms: i64) ![64]u8 {
    var random: [32]u8 = undefined;
    io.random(&random);
    const nonce = std.fmt.bytesToHex(random, .lower);
    const cwd = std.Io.Dir.cwd();
    _ = try cwd.createDirPathStatus(io, ".zconfig/runtime", @enumFromInt(0o700));
    const path = try std.fmt.allocPrint(allocator, ".zconfig/runtime/{s}.json", .{nonce});
    defer allocator.free(path);
    const data = try std.json.Stringify.valueAlloc(allocator, StoredCapability{ .nonce = &nonce, .source_digest = source_digest, .final_change_digest = final_digest, .issued_at_ms = now_ms, .expires_at_ms = now_ms + 5 * 60 * 1000 }, .{});
    defer allocator.free(data);
    var file = try cwd.createFile(io, path, .{ .exclusive = true, .permissions = @enumFromInt(0o600) });
    defer file.close(io);
    try file.writeStreamingAll(io, data);
    try file.sync(io);
    return nonce;
}

pub fn consumeCapability(allocator: std.mem.Allocator, io: std.Io, nonce: []const u8, source_digest: []const u8, final_digest: []const u8, now_ms: i64) !void {
    if (nonce.len != 64) return error.ConfirmationMismatch;
    for (nonce) |c| if (!std.ascii.isHex(c) or std.ascii.isUpper(c)) return error.ConfirmationMismatch;
    const cwd = std.Io.Dir.cwd();
    const path = try std.fmt.allocPrint(allocator, ".zconfig/runtime/{s}.json", .{nonce});
    defer allocator.free(path);
    var file = try cwd.openFile(io, path, .{});
    defer file.close(io);
    var buffer: [1024]u8 = undefined;
    var reader = file.reader(io, &buffer);
    const bytes = try reader.interface.allocRemaining(allocator, .limited(4096));
    defer allocator.free(bytes);
    var parsed = try protocol.parseStrict(StoredCapability, allocator, bytes);
    defer parsed.deinit();
    var capability = Capability{ .nonce = parsed.value.nonce, .source_digest = parsed.value.source_digest, .final_change_digest = parsed.value.final_change_digest, .issued_at_ms = parsed.value.issued_at_ms, .expires_at_ms = parsed.value.expires_at_ms };
    try capability.authorize(now_ms, nonce, source_digest, final_digest);
    try cwd.deleteFile(io, path);
}

pub fn assemble(allocator: std.mem.Allocator, source: []const u8, payload: protocol.FinalAssemblyPayload) !Assembly {
    var source_value = std.json.parseFromSlice(std.json.Value, allocator, source, .{}) catch return error.InvalidSource;
    defer source_value.deinit();
    try proposal.validateAgainstSource(allocator, payload.proposal, &source_value.value);
    const actual_source_digest = digest(source);
    if (!std.mem.eql(u8, &actual_source_digest, payload.source_digest) or !std.mem.eql(u8, payload.proposal.source_digest, payload.source_digest)) return error.StaleSource;
    const decisions = switch (payload.decisions) {
        .object => |v| v,
        else => return error.InvalidDecisions,
    };
    if (decisions.count() != payload.proposal.items.len) return error.DecisionSetMismatch;

    var edits: std.ArrayList(apply.Edit) = .empty;
    defer {
        for (edits.items) |edit| if (edit.value) |value| allocator.free(value);
        edits.deinit(allocator);
    }
    var ids: std.ArrayList([]const u8) = .empty;
    errdefer {
        for (ids.items) |id| allocator.free(id);
        ids.deinit(allocator);
    }
    for (payload.proposal.items) |item| {
        const decision_value = decisions.get(item.change_id) orelse return error.DecisionSetMismatch;
        const decision = switch (decision_value) {
            .string => |v| v,
            else => return error.InvalidDecisions,
        };
        if (std.mem.eql(u8, decision, "pending")) return error.PendingDecision;
        if (std.mem.eql(u8, decision, "rejected")) continue;
        if (!std.mem.eql(u8, decision, "approved")) return error.InvalidDecisions;
        for (payload.comments) |comment| if (std.mem.eql(u8, comment.change_id, item.change_id) and comment.status != .human_confirmed) return error.UnresolvedComment;
        const value = if (item.proposed_value) |v| try std.json.Stringify.valueAlloc(allocator, v, .{}) else null;
        try edits.append(allocator, .{ .path = item.path, .operation = switch (item.operation) {
            .add => .add,
            .replace => .replace,
            .remove => .remove,
        }, .value = value });
        try ids.append(allocator, try allocator.dupe(u8, item.change_id));
    }
    const candidate = try apply.compose(allocator, source, edits.items);
    errdefer allocator.free(candidate);
    const candidate_digest = digest(candidate);
    for (payload.external_checks) |check_set| try validateChecks(check_set, &candidate_digest);
    return .{ .allocator = allocator, .candidate = candidate, .approved_ids = try ids.toOwnedSlice(allocator), .final_digest = candidate_digest };
}

pub fn renderDiff(
    allocator: std.mem.Allocator,
    source: []const u8,
    candidate: []const u8,
    proposal_value: protocol.Proposal,
    approved_ids: []const []const u8,
    schema_root: ?std.json.Value,
) ![]u8 {
    var before_index = try source_index.Index.build(allocator, source);
    defer before_index.deinit();
    var after_index = try source_index.Index.build(allocator, candidate);
    defer after_index.deinit();
    var output: std.ArrayList(u8) = .empty;
    errdefer output.deinit(allocator);

    for (proposal_value.items) |item| {
        if (!containsID(approved_ids, item.change_id)) continue;
        const sensitive = redact.classify(item.path, if (schema_root) |root| try schema.isSensitiveAtPointer(allocator, root, item.path) else false) != .normal;
        const before = if (item.operation == .add) null else (before_index.find(item.path) orelse return error.DiffTargetMissing).bytes(source);
        const after = if (item.operation == .remove) null else (after_index.find(item.path) orelse return error.DiffTargetMissing).bytes(candidate);
        try output.appendSlice(allocator, "@@ ");
        try output.appendSlice(allocator, item.path);
        try output.appendSlice(allocator, " @@\n");
        if (before) |value| {
            try output.appendSlice(allocator, "- ");
            try output.appendSlice(allocator, if (sensitive) "[REDACTED]" else value);
            try output.append(allocator, '\n');
        }
        if (after) |value| {
            try output.appendSlice(allocator, "+ ");
            try output.appendSlice(allocator, if (sensitive) "[REDACTED]" else value);
            try output.append(allocator, '\n');
        }
    }
    return output.toOwnedSlice(allocator);
}

fn containsID(ids: []const []const u8, expected: []const u8) bool {
    for (ids) |id| if (std.mem.eql(u8, id, expected)) return true;
    return false;
}

fn validateChecks(value: std.json.Value, expected_digest: []const u8) !void {
    const object = switch (value) {
        .object => |v| v,
        else => return error.InvalidCheckResult,
    };
    const subject_value = object.get("subject_digest") orelse return error.InvalidCheckResult;
    const subject_digest = switch (subject_value) {
        .string => |v| v,
        else => return error.InvalidCheckResult,
    };
    if (!std.mem.eql(u8, subject_digest, expected_digest)) return error.CheckDigestMismatch;
    const checks_value = object.get("checks") orelse return error.InvalidCheckResult;
    const checks = switch (checks_value) {
        .array => |v| v,
        else => return error.InvalidCheckResult,
    };
    for (checks.items) |check| {
        const check_object = switch (check) {
            .object => |v| v,
            else => return error.InvalidCheckResult,
        };
        const status_value = check_object.get("status") orelse return error.InvalidCheckResult;
        const status = switch (status_value) {
            .string => |v| v,
            else => return error.InvalidCheckResult,
        };
        if (std.mem.eql(u8, status, "failed")) return error.MandatoryCheckFailed;
        if (!std.mem.eql(u8, status, "passed") and !std.mem.eql(u8, status, "unverified")) return error.InvalidCheckResult;
    }
}

pub fn digest(bytes: []const u8) [71]u8 {
    var hash: [32]u8 = undefined;
    std.crypto.hash.sha2.Sha256.hash(bytes, &hash, .{});
    var result: [71]u8 = undefined;
    @memcpy(result[0..7], "sha256:");
    const hex = std.fmt.bytesToHex(hash, .lower);
    @memcpy(result[7..], &hex);
    return result;
}
