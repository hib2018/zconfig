const std = @import("std");
const core = @import("zconfig_core");

pub fn main(init: std.process.Init) !void {
    const allocator = init.arena.allocator();
    const io = init.io;
    var input_buffer: [4096]u8 = undefined;
    var stdin = std.Io.File.stdin().reader(io, &input_buffer);
    const input = stdin.interface.allocRemaining(
        allocator,
        .limited(core.max_message_bytes + 1),
    ) catch |err| switch (err) {
        error.StreamTooLong => return writeFailure(io, "", "protocol.message_too_large", "Request exceeds 16 MiB."),
        else => return err,
    };

    var request = core.protocol.parseRequest(allocator, input) catch
        return writeFailure(io, "", "protocol.invalid_request", "Request is not a valid protocol message.");
    defer request.deinit();

    var output_buffer: [4096]u8 = undefined;
    var stdout = std.Io.File.stdout().writer(io, &output_buffer);
    switch (request.value.operation) {
        .protocol_info => try core.protocol.writeResponse(request.value, &stdout.interface),
        .inspect_source => writeInspection(allocator, io, request.value, &stdout.interface) catch
            return writeFailure(io, request.value.request_id, "source.invalid", "Source inspection failed."),
        .validate_proposal => writeProposalValidation(allocator, request.value, &stdout.interface) catch
            return writeFailure(io, request.value.request_id, "proposal.invalid", "Proposal validation failed."),
        .validate_revision => writeRevisionValidation(allocator, request.value, &stdout.interface) catch
            return writeFailure(io, request.value.request_id, "revision.scope_violation", "Revision changed an item outside the authorized scope."),
        .assemble_final => writeFinalAssembly(allocator, io, request.value, &stdout.interface) catch
            return writeFailure(io, request.value.request_id, "approval.not_ready", "Final change set is not ready for confirmation."),
        .apply_final => writeFinalApply(allocator, io, request.value, &stdout.interface) catch
            return writeFailure(io, request.value.request_id, "apply.rejected", "Final confirmation or source validation failed."),
    }
    try stdout.interface.writeByte('\n');
    try stdout.flush();
}

fn nowMs(io: std.Io) i64 {
    return @intCast(@divFloor(std.Io.Clock.real.now(io).nanoseconds, std.time.ns_per_ms));
}

fn writeFinalAssembly(allocator: std.mem.Allocator, io: std.Io, request: core.protocol.Request, writer: *std.Io.Writer) !void {
    var payload = try std.json.parseFromValue(core.protocol.FinalAssemblyPayload, allocator, request.payload, .{});
    defer payload.deinit();
    var loaded = try core.document.load(allocator, io, payload.value.source_path);
    defer loaded.deinit(allocator);
    var assembled = try core.final_set.assemble(allocator, loaded.bytes, payload.value);
    defer assembled.deinit();
    var loaded_schema: ?core.document.Loaded = if (payload.value.schema_path) |path|
        try core.document.load(allocator, io, path)
    else
        null;
    defer if (loaded_schema) |*value| value.deinit(allocator);
    const schema_root: ?std.json.Value = if (loaded_schema) |*value| value.document.parsed.value else null;
    const diff = try core.final_set.renderDiff(allocator, loaded.bytes, assembled.candidate, payload.value.proposal, assembled.approved_ids, schema_root);
    defer allocator.free(diff);
    const nonce = try core.final_set.issueCapability(allocator, io, payload.value.source_digest, &assembled.final_digest, nowMs(io));
    try std.json.Stringify.value(.{
        .protocol_version = core.protocol_version,
        .request_id = request.request_id,
        .ok = true,
        .result_schema = "zconfig.final-change/1",
        .result = .{ .final_change_digest = &assembled.final_digest, .source_digest = payload.value.source_digest, .approved_change_item_ids = assembled.approved_ids, .diff = diff, .checks = &.{}, .confirmation_nonce = &nonce },
    }, .{}, writer);
}

fn writeFinalApply(allocator: std.mem.Allocator, io: std.Io, request: core.protocol.Request, writer: *std.Io.Writer) !void {
    var payload = try std.json.parseFromValue(core.protocol.ApplyPayload, allocator, request.payload, .{});
    defer payload.deinit();
    var loaded = try core.document.load(allocator, io, payload.value.source_path);
    defer loaded.deinit(allocator);
    const assembly_payload = core.protocol.FinalAssemblyPayload{ .source_path = payload.value.source_path, .schema_path = payload.value.schema_path, .source_digest = payload.value.source_digest, .proposal = payload.value.proposal, .decisions = payload.value.decisions, .comments = payload.value.comments, .external_checks = payload.value.external_checks };
    var assembled = try core.final_set.assemble(allocator, loaded.bytes, assembly_payload);
    defer assembled.deinit();
    if (!std.mem.eql(u8, &assembled.final_digest, payload.value.final_change_digest)) return error.FinalDigestMismatch;
    try core.final_set.consumeCapability(allocator, io, payload.value.confirmation_nonce, payload.value.source_digest, payload.value.final_change_digest, nowMs(io));
    const result = try core.apply.replaceCandidateInDir(allocator, io, std.Io.Dir.cwd(), payload.value.source_path, payload.value.source_digest, assembled.candidate);
    const recovery_path = try std.fmt.allocPrint(allocator, "{s}.zconfig-recovery", .{payload.value.source_path});
    try std.json.Stringify.value(.{
        .protocol_version = core.protocol_version,
        .request_id = request.request_id,
        .ok = true,
        .result_schema = "zconfig.apply/1",
        .result = .{ .source_digest_before = &result.source_digest_before, .source_digest_after = &result.source_digest_after, .outcome = "applied", .recovery = .{ .available = result.recovery_available, .path = recovery_path } },
    }, .{}, writer);
}

fn writeRevisionValidation(allocator: std.mem.Allocator, request: core.protocol.Request, writer: *std.Io.Writer) !void {
    var payload = try std.json.parseFromValue(core.protocol.RevisionValidationPayload, allocator, request.payload, .{});
    defer payload.deinit();
    try core.proposal.validateRevision(payload.value.active_proposal, payload.value.candidate_proposal, payload.value.allowed_change_item_ids);
    try std.json.Stringify.value(.{
        .protocol_version = core.protocol_version,
        .request_id = request.request_id,
        .ok = true,
        .result_schema = "zconfig.validation-result/1",
        .result = .{
            .subject_digest = payload.value.candidate_proposal.source_digest,
            .checks = &.{.{ .kind = "revision_scope", .status = "passed", .code = "revision.scope_valid", .message = "Revision is confined to commented change items." }},
        },
    }, .{}, writer);
}

fn writeInspection(allocator: std.mem.Allocator, io: std.Io, request: core.protocol.Request, writer: *std.Io.Writer) !void {
    var payload = try std.json.parseFromValue(core.protocol.InspectSourcePayload, allocator, request.payload, .{});
    defer payload.deinit();
    var loaded = try core.document.load(allocator, io, payload.value.source_path);
    defer loaded.deinit(allocator);
    var loaded_schema: ?core.document.Loaded = if (payload.value.schema_path) |path|
        try core.document.load(allocator, io, path)
    else
        null;
    defer if (loaded_schema) |*value| value.deinit(allocator);
    const schema_root: ?std.json.Value = if (loaded_schema) |*value| value.document.parsed.value else null;
    var index = try core.source_index.Index.build(allocator, loaded.bytes);
    defer index.deinit();

    var settings = std.json.Array.init(allocator);
    for (index.nodes, 0..) |node, i| {
        var parsed_pointer = try core.pointer.parse(allocator, node.pointer);
        defer parsed_pointer.deinit();
        const resolved = try core.pointer.resolve(&loaded.document.parsed.value, parsed_pointer, .existing);
        const item_schema = if (schema_root) |root| schemaAtPointer(root, parsed_pointer.tokens) else null;
        var sensitivity = core.redact.classify(node.pointer, if (item_schema) |value| core.schema.isSensitive(value) else false);
        if (sensitivity == .normal and try hasSensitiveDescendant(allocator, index.nodes, node.pointer, schema_root))
            sensitivity = .suspected_sensitive;
        var value_object: std.json.ObjectMap = .empty;
        if (sensitivity == .normal) {
            try value_object.put(allocator, "visibility", .{ .string = "plain" });
            try value_object.put(allocator, "value", resolved.value.*);
        } else {
            const encoded = node.bytes(loaded.bytes);
            const fp = core.redact.fingerprint(encoded, loaded.document.digest());
            const fingerprint = try allocator.dupe(u8, &fp);
            const secret_ref = try std.fmt.allocPrint(allocator, "secret-{d}", .{i});
            try value_object.put(allocator, "visibility", .{ .string = "redacted" });
            try value_object.put(allocator, "secret_ref", .{ .string = secret_ref });
            try value_object.put(allocator, "value_type", .{ .string = @tagName(core.redact.valueType(resolved.value.*)) });
            try value_object.put(allocator, "fingerprint", .{ .string = fingerprint });
        }
        var setting: std.json.ObjectMap = .empty;
        try setting.put(allocator, "path", .{ .string = node.pointer });
        try setting.put(allocator, "value", .{ .object = value_object });
        try setting.put(allocator, "sensitivity", .{ .string = @tagName(sensitivity) });
        try settings.append(.{ .object = setting });
    }
    const result = .{
        .source_digest = loaded.document.digest(),
        .byte_length = loaded.document.byte_length,
        .node_count = loaded.document.node_count,
        .settings = settings.items,
    };
    try std.json.Stringify.value(.{
        .protocol_version = core.protocol_version,
        .request_id = request.request_id,
        .ok = true,
        .result_schema = "zconfig.inspection/1",
        .result = result,
    }, .{}, writer);
}

fn hasSensitiveDescendant(allocator: std.mem.Allocator, nodes: []const core.source_index.Node, parent: []const u8, schema_root: ?std.json.Value) !bool {
    for (nodes) |candidate| {
        if (candidate.pointer.len <= parent.len) continue;
        const below = if (parent.len == 0)
            candidate.pointer[0] == '/'
        else
            std.mem.startsWith(u8, candidate.pointer, parent) and candidate.pointer[parent.len] == '/';
        if (!below) continue;
        var parsed = try core.pointer.parse(allocator, candidate.pointer);
        defer parsed.deinit();
        const item_schema = if (schema_root) |root| schemaAtPointer(root, parsed.tokens) else null;
        if (core.redact.classify(candidate.pointer, if (item_schema) |value| core.schema.isSensitive(value) else false) != .normal) return true;
    }
    return false;
}

fn schemaAtPointer(root: std.json.Value, tokens: []const []const u8) ?std.json.Value {
    var current = root;
    for (tokens) |token| {
        if (current != .object) return null;
        if (current.object.get("properties")) |properties| {
            if (properties == .object) {
                if (properties.object.get(token)) |child| {
                    current = child;
                    continue;
                }
            }
        }
        if (current.object.get("items")) |items| {
            current = items;
            continue;
        }
        return null;
    }
    return current;
}

fn writeProposalValidation(allocator: std.mem.Allocator, request: core.protocol.Request, writer: *std.Io.Writer) !void {
    var parsed = try std.json.parseFromValue(core.protocol.Proposal, allocator, request.payload, .{});
    defer parsed.deinit();
    try core.proposal.validate(parsed.value);
    const check = .{ .kind = "proposal", .status = "passed", .code = "proposal.valid", .message = "Proposal is structurally valid." };
    try std.json.Stringify.value(.{
        .protocol_version = core.protocol_version,
        .request_id = request.request_id,
        .ok = true,
        .result_schema = "zconfig.validation-result/1",
        .result = .{ .subject_digest = parsed.value.source_digest, .checks = &.{check} },
    }, .{}, writer);
}

fn writeFailure(io: std.Io, request_id: []const u8, code: []const u8, message: []const u8) !void {
    var output_buffer: [4096]u8 = undefined;
    var stdout = std.Io.File.stdout().writer(io, &output_buffer);
    const response = core.protocol.Failure{
        .request_id = request_id,
        .@"error" = .{ .code = code, .message = message, .retryable = false },
    };
    try std.json.Stringify.value(response, .{}, &stdout.interface);
    try stdout.interface.writeByte('\n');
    try stdout.flush();
}
