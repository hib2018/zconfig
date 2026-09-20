const std = @import("std");
const protocol = @import("protocol.zig");
const proposal = @import("proposal.zig");

const Check = struct { kind: []const u8, status: []const u8, code: []const u8, message: []const u8, pointer: ?[]const u8 = null, evidence_digest: ?[]const u8 = null };
const Validation = struct { subject_digest: []const u8, checks: []const Check };
const Visible = struct { visibility: []const u8, value: ?std.json.Value = null, secret_ref: ?[]const u8 = null, value_type: ?[]const u8 = null, fingerprint: ?[]const u8 = null };
const Setting = struct { path: []const u8, value: Visible, sensitivity: []const u8, title: ?[]const u8 = null, description: ?[]const u8 = null };
const RevisionResult = struct { proposal_id: []const u8, base_revision: u64, source_digest: []const u8, proposal: protocol.Proposal, resolved_comment_ids: []const []const u8 };
const FinalResult = struct { final_change_digest: []const u8, source_digest: []const u8, approved_change_item_ids: []const []const u8, diff: []const u8, checks: []const Check, confirmation_nonce: []const u8 };
const ApplyResult = struct { source_digest_before: []const u8, source_digest_after: []const u8, outcome: []const u8, recovery: struct { available: bool, path: ?[]const u8 = null } };
const ValidatorRequest = struct { candidate_path: []const u8, candidate_digest: []const u8, source_digest: []const u8 };
const ValidatorResult = struct { candidate_digest: []const u8, status: []const u8, code: []const u8, message: []const u8, details: ?[]const std.json.Value = null };
const Audit = struct {
    audit_schema: []const u8,
    event_id: []const u8,
    session_id: []const u8,
    sequence: u64,
    occurred_at: []const u8,
    actor_type: []const u8,
    action: []const u8,
    change_ids: []const []const u8,
    comment_ids: ?[]const []const u8 = null,
    source_digest: []const u8,
    proposal_digest: ?[]const u8 = null,
    outcome_code: ?[]const u8 = null,
    previous_event_digest: ?[]const u8,
    event_digest: []const u8,
};

pub fn validate(kind: []const u8, allocator: std.mem.Allocator, bytes: []const u8, expected_digest: ?[]const u8) !void {
    if (std.mem.eql(u8, kind, "envelope")) return validateEnvelope(allocator, bytes);
    if (std.mem.eql(u8, kind, "empty")) {
        var parsed = try protocol.parseStrict(struct {}, allocator, bytes);
        parsed.deinit();
        return;
    }
    if (std.mem.eql(u8, kind, "proposal")) {
        var parsed = try proposal.parse(allocator, bytes);
        parsed.deinit();
        return;
    }
    if (std.mem.eql(u8, kind, "setting")) return validateSetting(allocator, bytes);
    if (std.mem.eql(u8, kind, "protocol_info")) return validateProtocolInfo(allocator, bytes);
    if (std.mem.eql(u8, kind, "inspection_request")) {
        var parsed = try protocol.parseStrict(protocol.InspectSourcePayload, allocator, bytes);
        defer parsed.deinit();
        if (parsed.value.source_path.len == 0) return error.InvalidContract;
        return;
    }
    if (std.mem.eql(u8, kind, "inspection_result")) return validateInspection(allocator, bytes);
    if (std.mem.eql(u8, kind, "revision_request")) return validateRevisionRequest(allocator, bytes);
    if (std.mem.eql(u8, kind, "revision_result")) return validateRevisionResult(allocator, bytes);
    if (std.mem.eql(u8, kind, "validation_result")) return validateValidationDocument(allocator, bytes);
    if (std.mem.eql(u8, kind, "final_request")) return validateFinalRequest(allocator, bytes);
    if (std.mem.eql(u8, kind, "final_result")) return validateFinalResult(allocator, bytes);
    if (std.mem.eql(u8, kind, "apply_request")) return validateApplyRequest(allocator, bytes);
    if (std.mem.eql(u8, kind, "apply_result")) return validateApplyResult(allocator, bytes);
    if (std.mem.eql(u8, kind, "audit_event")) return validateAudit(allocator, bytes);
    if (std.mem.eql(u8, kind, "validator_request")) return validateValidatorRequest(allocator, bytes);
    if (std.mem.eql(u8, kind, "validator_result")) return validateValidatorResult(allocator, bytes, expected_digest);
    return error.UnknownContract;
}

fn validateEnvelope(allocator: std.mem.Allocator, bytes: []const u8) !void {
    var shape = try protocol.parseStrict(std.json.Value, allocator, bytes);
    defer shape.deinit();
    if (shape.value != .object) return error.InvalidContract;
    if (shape.value.object.contains("operation")) {
        var request = try protocol.parseRequest(allocator, bytes);
        request.deinit();
        return;
    }
    try allowed(&shape.value.object, &.{ "protocol_version", "request_id", "ok", "result_schema", "result", "error" });
    if (!stringEq(shape.value.object.get("protocol_version"), protocol.protocol_version) or getString(shape.value.object.get("request_id")) == null) return error.InvalidContract;
    const ok = getBool(shape.value.object.get("ok")) orelse return error.InvalidContract;
    if (ok) {
        if (getString(shape.value.object.get("result_schema")) == null or !shape.value.object.contains("result") or shape.value.object.contains("error")) return error.InvalidContract;
    } else {
        if (shape.value.object.contains("result") or shape.value.object.contains("result_schema")) return error.InvalidContract;
        const error_value = shape.value.object.get("error") orelse return error.InvalidContract;
        if (error_value != .object) return error.InvalidContract;
        try allowed(&error_value.object, &.{ "code", "message", "retryable", "details" });
        const code = getString(error_value.object.get("code")) orelse return error.InvalidContract;
        if (std.mem.indexOfScalar(u8, code, '.') == null or getString(error_value.object.get("message")) == null or getBool(error_value.object.get("retryable")) == null) return error.InvalidContract;
    }
}

fn validateProtocolInfo(allocator: std.mem.Allocator, bytes: []const u8) !void {
    var parsed = try protocol.parseStrict(protocol.ProtocolInfo, allocator, bytes);
    defer parsed.deinit();
    if (parsed.value.protocol_versions.len == 0 or parsed.value.operations.len == 0 or parsed.value.schemas.len == 0 or parsed.value.limits.message_bytes == 0) return error.InvalidContract;
}

fn validateSetting(allocator: std.mem.Allocator, bytes: []const u8) !void {
    var parsed = try protocol.parseStrict(Setting, allocator, bytes);
    defer parsed.deinit();
    const value = parsed.value;
    if (!proposal.validPointer(value.path) or !(std.mem.eql(u8, value.sensitivity, "normal") or std.mem.eql(u8, value.sensitivity, "schema_sensitive") or std.mem.eql(u8, value.sensitivity, "suspected_sensitive"))) return error.InvalidContract;
    if (std.mem.eql(u8, value.value.visibility, "plain")) {
        if (value.value.value == null or value.value.secret_ref != null or value.value.value_type != null or value.value.fingerprint != null) return error.InvalidContract;
    } else if (std.mem.eql(u8, value.value.visibility, "redacted")) {
        if (value.value.value != null or value.value.secret_ref == null or value.value.value_type == null or value.value.fingerprint == null) return error.InvalidContract;
        const redacted = protocol.RedactedValue{ .visibility = .redacted, .secret_ref = value.value.secret_ref.?, .value_type = std.meta.stringToEnum(protocol.ValueType, value.value.value_type.?) orelse return error.InvalidContract, .fingerprint = value.value.fingerprint.? };
        if (!redacted.isValid()) return error.InvalidContract;
    } else return error.InvalidContract;
}

fn validateInspection(allocator: std.mem.Allocator, bytes: []const u8) !void {
    const T = struct { source_digest: []const u8, byte_length: i64, node_count: i64, settings: []const Setting };
    var parsed = try protocol.parseStrict(T, allocator, bytes);
    defer parsed.deinit();
    if (!proposal.validDigest(parsed.value.source_digest) or parsed.value.byte_length < 0 or parsed.value.byte_length > 10 * 1024 * 1024 or parsed.value.node_count < 1 or parsed.value.node_count > 100_000) return error.InvalidContract;
    for (parsed.value.settings) |setting| {
        const encoded = try std.json.Stringify.valueAlloc(allocator, setting, .{});
        defer allocator.free(encoded);
        try validateSetting(allocator, encoded);
    }
}

fn validateRevisionRequest(allocator: std.mem.Allocator, bytes: []const u8) !void {
    var parsed = try protocol.parseStrict(protocol.RevisionPayload, allocator, bytes);
    defer parsed.deinit();
    const value = parsed.value;
    if (value.proposal_id.len == 0 or value.base_revision == 0 or !proposal.validDigest(value.source_digest) or value.allowed_change_item_ids.len == 0 or value.comments.len == 0 or !unique(value.allowed_change_item_ids)) return error.InvalidContract;
    for (value.comments) |comment| if (comment.comment_id.len == 0 or comment.change_id.len == 0 or comment.body.len == 0) return error.InvalidContract;
    try proposal.validate(value.proposal);
}

fn validateRevisionResult(allocator: std.mem.Allocator, bytes: []const u8) !void {
    var parsed = try protocol.parseStrict(RevisionResult, allocator, bytes);
    defer parsed.deinit();
    if (parsed.value.proposal_id.len == 0 or parsed.value.base_revision == 0 or !proposal.validDigest(parsed.value.source_digest) or !unique(parsed.value.resolved_comment_ids)) return error.InvalidContract;
    try proposal.validate(parsed.value.proposal);
}

fn validateChecks(checks: []const Check) !void {
    for (checks) |check| {
        if (check.kind.len == 0 or check.code.len == 0 or check.message.len > 4096) return error.InvalidContract;
        if (!(std.mem.eql(u8, check.status, "passed") or std.mem.eql(u8, check.status, "failed") or std.mem.eql(u8, check.status, "unverified"))) return error.InvalidContract;
    }
}

fn validateValidationDocument(allocator: std.mem.Allocator, bytes: []const u8) !void {
    var parsed = try protocol.parseStrict(Validation, allocator, bytes);
    defer parsed.deinit();
    if (!proposal.validDigest(parsed.value.subject_digest)) return error.InvalidContract;
    try validateChecks(parsed.value.checks);
}

fn validateFinalRequest(allocator: std.mem.Allocator, bytes: []const u8) !void {
    var parsed = try protocol.parseStrict(protocol.FinalAssemblyPayload, allocator, bytes);
    defer parsed.deinit();
    if (parsed.value.source_path.len == 0 or !proposal.validDigest(parsed.value.source_digest)) return error.InvalidContract;
    try proposal.validate(parsed.value.proposal);
    const decisions = if (parsed.value.decisions == .object) parsed.value.decisions.object else return error.InvalidContract;
    var it = decisions.iterator();
    while (it.next()) |entry| {
        if (entry.value_ptr.* != .string) return error.InvalidContract;
        const decision = entry.value_ptr.string;
        if (!(std.mem.eql(u8, decision, "pending") or std.mem.eql(u8, decision, "approved") or std.mem.eql(u8, decision, "rejected"))) return error.InvalidContract;
    }
    for (parsed.value.external_checks) |check| {
        const encoded = try std.json.Stringify.valueAlloc(allocator, check, .{});
        defer allocator.free(encoded);
        try validateValidationDocument(allocator, encoded);
    }
}

fn validateFinalResult(allocator: std.mem.Allocator, bytes: []const u8) !void {
    var parsed = try protocol.parseStrict(FinalResult, allocator, bytes);
    defer parsed.deinit();
    if (!proposal.validDigest(parsed.value.final_change_digest) or !proposal.validDigest(parsed.value.source_digest) or parsed.value.confirmation_nonce.len < 32 or !unique(parsed.value.approved_change_item_ids)) return error.InvalidContract;
    try validateChecks(parsed.value.checks);
}

fn validateApplyRequest(allocator: std.mem.Allocator, bytes: []const u8) !void {
    var parsed = try protocol.parseStrict(protocol.ApplyPayload, allocator, bytes);
    defer parsed.deinit();
    const value = parsed.value;
    if (value.source_path.len == 0 or !proposal.validDigest(value.source_digest) or !proposal.validDigest(value.final_change_digest) or value.confirmation_nonce.len < 32) return error.InvalidContract;
    try proposal.validate(value.proposal);
    const decisions = if (value.decisions == .object) value.decisions.object else return error.InvalidContract;
    var it = decisions.iterator();
    while (it.next()) |entry| {
        if (entry.value_ptr.* != .string) return error.InvalidContract;
        const decision = entry.value_ptr.string;
        if (!(std.mem.eql(u8, decision, "pending") or std.mem.eql(u8, decision, "approved") or std.mem.eql(u8, decision, "rejected"))) return error.InvalidContract;
    }
}

fn validateApplyResult(allocator: std.mem.Allocator, bytes: []const u8) !void {
    var parsed = try protocol.parseStrict(ApplyResult, allocator, bytes);
    defer parsed.deinit();
    if (!proposal.validDigest(parsed.value.source_digest_before) or !proposal.validDigest(parsed.value.source_digest_after)) return error.InvalidContract;
    if (!(std.mem.eql(u8, parsed.value.outcome, "applied") or std.mem.eql(u8, parsed.value.outcome, "recovered") or std.mem.eql(u8, parsed.value.outcome, "unchanged"))) return error.InvalidContract;
}

fn validateAudit(allocator: std.mem.Allocator, bytes: []const u8) !void {
    var parsed = try protocol.parseStrict(Audit, allocator, bytes);
    defer parsed.deinit();
    const value = parsed.value;
    if (!std.mem.eql(u8, value.audit_schema, "zconfig.audit-event/1") or value.event_id.len == 0 or value.session_id.len == 0 or value.sequence == 0 or value.sequence > 5000 or !proposal.validDigest(value.source_digest) or !proposal.validDigest(value.event_digest) or !unique(value.change_ids)) return error.InvalidContract;
    if (value.comment_ids) |ids| if (!unique(ids)) return error.InvalidContract;
    if (value.proposal_digest) |digest| if (!proposal.validDigest(digest)) return error.InvalidContract;
    if (value.previous_event_digest) |digest| if (!proposal.validDigest(digest)) return error.InvalidContract;
}

fn validateValidatorRequest(allocator: std.mem.Allocator, bytes: []const u8) !void {
    var parsed = try protocol.parseStrict(ValidatorRequest, allocator, bytes);
    defer parsed.deinit();
    if (parsed.value.candidate_path.len == 0 or !proposal.validDigest(parsed.value.candidate_digest) or !proposal.validDigest(parsed.value.source_digest)) return error.InvalidContract;
}

fn validateValidatorResult(allocator: std.mem.Allocator, bytes: []const u8, expected: ?[]const u8) !void {
    var parsed = try protocol.parseStrict(ValidatorResult, allocator, bytes);
    defer parsed.deinit();
    const value = parsed.value;
    if (!proposal.validDigest(value.candidate_digest) or value.code.len == 0 or value.message.len > 4096) return error.InvalidContract;
    if (expected) |digest| if (!std.mem.eql(u8, value.candidate_digest, digest)) return error.InvalidContract;
    if (!(std.mem.eql(u8, value.status, "passed") or std.mem.eql(u8, value.status, "failed") or std.mem.eql(u8, value.status, "unverified"))) return error.InvalidContract;
}

fn allowed(object: *const std.json.ObjectMap, keys: []const []const u8) !void {
    var it = object.iterator();
    while (it.next()) |entry| {
        var found = false;
        for (keys) |key| if (std.mem.eql(u8, entry.key_ptr.*, key)) {
            found = true;
            break;
        };
        if (!found) return error.UnknownField;
    }
}
fn getString(value: ?std.json.Value) ?[]const u8 {
    const v = value orelse return null;
    return if (v == .string) v.string else null;
}
fn getBool(value: ?std.json.Value) ?bool {
    const v = value orelse return null;
    return if (v == .bool) v.bool else null;
}
fn stringEq(value: ?std.json.Value, expected: []const u8) bool {
    const actual = getString(value) orelse return false;
    return std.mem.eql(u8, actual, expected);
}
fn unique(values: []const []const u8) bool {
    for (values, 0..) |value, i| for (values[0..i]) |previous| if (std.mem.eql(u8, value, previous)) return false;
    return true;
}
