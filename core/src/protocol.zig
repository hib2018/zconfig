const std = @import("std");

pub const protocol_version = "1.0";
pub const max_message_bytes: usize = 16 * 1024 * 1024;

pub const Operation = enum {
    protocol_info,
    inspect_source,
    validate_proposal,
    validate_revision,
    assemble_final,
    apply_final,
};

pub const Request = struct {
    protocol_version: []const u8,
    request_id: []const u8,
    operation: Operation,
    payload_schema: []const u8,
    payload: std.json.Value,
};

pub const ErrorDetail = struct { pointer: ?[]const u8 = null, reason: []const u8 };
pub const ProtocolError = struct {
    code: []const u8,
    message: []const u8,
    retryable: bool,
    details: ?[]const ErrorDetail = null,
};
pub const Success = struct {
    protocol_version: []const u8 = protocol_version,
    request_id: []const u8,
    ok: bool = true,
    result_schema: []const u8,
    result: std.json.Value,
};
pub const Failure = struct {
    protocol_version: []const u8 = protocol_version,
    request_id: []const u8,
    ok: bool = false,
    @"error": ProtocolError,
};

pub const ValueType = enum { string, number, integer, boolean, null, array, object };
pub const RedactedValue = struct {
    visibility: enum { redacted },
    secret_ref: []const u8,
    value_type: ValueType,
    fingerprint: []const u8,

    pub fn isValid(self: RedactedValue) bool {
        const prefix = "hmac-sha256:";
        if (self.secret_ref.len == 0 or self.fingerprint.len != prefix.len + 64 or
            !std.mem.startsWith(u8, self.fingerprint, prefix)) return false;
        for (self.fingerprint[prefix.len..]) |c|
            if (!std.ascii.isHex(c) or std.ascii.isUpper(c)) return false;
        return true;
    }
};

pub const ProtocolInfo = struct {
    protocol_versions: []const []const u8,
    operations: []const []const u8,
    schemas: []const []const u8,
    limits: struct { message_bytes: usize },
};

pub const ParsedRequest = std.json.Parsed(Request);

pub fn parseStrict(comptime T: type, allocator: std.mem.Allocator, bytes: []const u8) !std.json.Parsed(T) {
    if (bytes.len > max_message_bytes) return error.MessageTooLarge;
    return std.json.parseFromSlice(T, allocator, bytes, .{}) catch |err| return switch (err) {
        error.DuplicateField => error.DuplicateField,
        error.UnknownField => error.UnknownField,
        else => error.InvalidJson,
    };
}

pub fn parseRequest(allocator: std.mem.Allocator, bytes: []const u8) !ParsedRequest {
    if (bytes.len > max_message_bytes) return error.MessageTooLarge;
    var parsed = try parseStrict(Request, allocator, bytes);
    errdefer parsed.deinit();
    const request = parsed.value;
    if (!std.mem.eql(u8, request.protocol_version, protocol_version))
        return error.UnsupportedProtocolVersion;
    if (request.request_id.len == 0 or request.request_id.len > 128)
        return error.InvalidRequestId;
    try validatePayload(request.operation, request.payload_schema, request.payload);
    return parsed;
}

pub const InspectSourcePayload = struct {
    source_path: []const u8,
    schema_path: ?[]const u8 = null,
};

pub const ChangeOperation = enum { add, replace, remove };
pub const ChangeItem = struct {
    change_id: []const u8,
    path: []const u8,
    operation: ChangeOperation,
    expected_old: ?std.json.Value = null,
    proposed_value: ?std.json.Value = null,
    explanation: []const u8,
};
pub const Proposal = struct {
    proposal_id: []const u8,
    revision: u64,
    source_digest: []const u8,
    created_at: []const u8,
    producer: ?[]const u8 = null,
    items: []const ChangeItem,
};
pub const ReviewComment = struct {
    comment_id: []const u8,
    change_id: []const u8,
    body: []const u8,
};
pub const RevisionPayload = struct {
    proposal_id: []const u8,
    base_revision: u64,
    source_digest: []const u8,
    allowed_change_item_ids: []const []const u8,
    comments: []const ReviewComment,
    proposal: Proposal,
    source_context: ?std.json.Value = null,
    authorized_secrets: ?std.json.Value = null,
};
pub const RevisionValidationPayload = struct {
    active_proposal: Proposal,
    candidate_proposal: Proposal,
    allowed_change_item_ids: []const []const u8,
    resolved_comment_ids: []const []const u8,
};
pub const FinalAssemblyPayload = struct {
    source_path: []const u8,
    source_digest: []const u8,
    proposal: Proposal,
    decisions: std.json.Value,
    comments: []const ReviewComment,
    external_checks: []const std.json.Value,
};
pub const ApplyPayload = struct {
    source_path: []const u8,
    source_digest: []const u8,
    final_change_digest: []const u8,
    confirmation_nonce: []const u8,
};

fn validatePayload(operation: Operation, schema: []const u8, payload: std.json.Value) !void {
    const expected = switch (operation) {
        .protocol_info => "zconfig.empty/1",
        .inspect_source => "zconfig.inspection/1",
        .validate_proposal => "zconfig.proposal/1",
        .validate_revision => "zconfig.revision/1",
        .assemble_final => "zconfig.final-change/1",
        .apply_final => "zconfig.apply/1",
    };
    if (!std.mem.eql(u8, schema, expected)) return error.InvalidPayloadSchema;
    if (operation == .protocol_info) {
        const object = switch (payload) {
            .object => |value| value,
            else => return error.InvalidOperationPayload,
        };
        if (object.count() != 0) return error.InvalidOperationPayload;
    }
}

pub fn sanitizeOutput(allocator: std.mem.Allocator, input: []const u8, secrets: []const []const u8) ![]u8 {
    var current = try allocator.dupe(u8, input);
    errdefer allocator.free(current);
    for (secrets) |secret| {
        if (secret.len == 0) continue;
        const replaced = try std.mem.replaceOwned(u8, allocator, current, secret, "[REDACTED]");
        allocator.free(current);
        current = replaced;
    }
    return current;
}

pub fn capabilityInfo() ProtocolInfo {
    return .{
        .protocol_versions = &.{protocol_version},
        .operations = &.{ "protocol_info", "inspect_source", "validate_proposal", "validate_revision", "assemble_final", "apply_final" },
        .schemas = &.{ "zconfig.empty/1", "zconfig.protocol-info/1", "zconfig.inspection/1", "zconfig.proposal/1", "zconfig.revision/1", "zconfig.final-change/1", "zconfig.apply/1" },
        .limits = .{ .message_bytes = max_message_bytes },
    };
}

pub fn writeResponse(request: Request, writer: *std.Io.Writer) !void {
    if (request.operation == .protocol_info) {
        const response = .{
            .protocol_version = protocol_version,
            .request_id = request.request_id,
            .ok = true,
            .result_schema = "zconfig.protocol-info/1",
            .result = capabilityInfo(),
        };
        return std.json.Stringify.value(response, .{}, writer);
    }
    const response = Failure{
        .request_id = request.request_id,
        .@"error" = .{
            .code = "protocol.operation_unavailable",
            .message = "Operation is recognized but not implemented.",
            .retryable = false,
        },
    };
    try std.json.Stringify.value(response, .{}, writer);
}
