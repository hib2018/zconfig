const std = @import("std");
const protocol = @import("zconfig_core").protocol;

test "strict request framing and envelope fields" {
    const allocator = std.testing.allocator;
    var request = try protocol.parseRequest(allocator,
        \\{"protocol_version":"1.0","request_id":"req-1","operation":"protocol_info","payload_schema":"zconfig.empty/1","payload":{}}
    );
    defer request.deinit();
    try std.testing.expectEqual(protocol.Operation.protocol_info, request.value.operation);

    try std.testing.expectError(error.DuplicateField, protocol.parseRequest(allocator,
        \\{"protocol_version":"1.0","request_id":"a","request_id":"b","operation":"protocol_info","payload_schema":"zconfig.empty/1","payload":{}}
    ));
    try std.testing.expectError(error.UnknownField, protocol.parseRequest(allocator,
        \\{"protocol_version":"1.0","request_id":"a","operation":"protocol_info","payload_schema":"zconfig.empty/1","payload":{},"extra":true}
    ));
    try std.testing.expectError(error.InvalidJson, protocol.parseRequest(allocator,
        \\{"protocol_version":"1.0","request_id":"a","operation":"protocol_info","payload_schema":"zconfig.empty/1","payload":{}} {}
    ));
}

test "versions request ids and size limits are enforced" {
    const allocator = std.testing.allocator;
    try std.testing.expectError(error.UnsupportedProtocolVersion, protocol.parseRequest(allocator,
        \\{"protocol_version":"2.0","request_id":"a","operation":"protocol_info","payload_schema":"zconfig.empty/1","payload":{}}
    ));
    try std.testing.expectError(error.InvalidRequestId, protocol.parseRequest(allocator,
        \\{"protocol_version":"1.0","request_id":"","operation":"protocol_info","payload_schema":"zconfig.empty/1","payload":{}}
    ));
    try std.testing.expectError(error.InvalidPayloadSchema, protocol.parseRequest(allocator,
        \\{"protocol_version":"1.0","request_id":"a","operation":"protocol_info","payload_schema":"zconfig.wrong/1","payload":{}}
    ));
    const oversized = try allocator.alloc(u8, protocol.max_message_bytes + 1);
    defer allocator.free(oversized);
    @memset(oversized, ' ');
    try std.testing.expectError(error.MessageTooLarge, protocol.parseRequest(allocator, oversized));
}

test "all core operations negotiate their document schema" {
    const allocator = std.testing.allocator;
    const cases = [_]struct { operation: []const u8, schema: []const u8, payload: []const u8 }{
        .{ .operation = "protocol_info", .schema = "zconfig.empty/1", .payload = "{}" },
        .{ .operation = "inspect_source", .schema = "zconfig.inspection/1", .payload = "{\"source_path\":\"app.json\"}" },
        .{ .operation = "validate_proposal", .schema = "zconfig.proposal/1", .payload = "{}" },
        .{ .operation = "validate_revision", .schema = "zconfig.revision/1", .payload = "{}" },
        .{ .operation = "assemble_final", .schema = "zconfig.final-change/1", .payload = "{}" },
        .{ .operation = "apply_final", .schema = "zconfig.apply/1", .payload = "{}" },
    };
    for (cases) |case| {
        const input = try std.fmt.allocPrint(allocator, "{{\"protocol_version\":\"1.0\",\"request_id\":\"r\",\"operation\":\"{s}\",\"payload_schema\":\"{s}\",\"payload\":{s}}}", .{ case.operation, case.schema, case.payload });
        defer allocator.free(input);
        var request = try protocol.parseRequest(allocator, input);
        request.deinit();
    }
}

test "typed redaction and output sanitization" {
    try std.testing.expect((protocol.RedactedValue{
        .visibility = .redacted,
        .secret_ref = "secret-1",
        .value_type = .string,
        .fingerprint = "hmac-sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    }).isValid());
    const secrets = [_][]const u8{"very-secret"};
    const clean = try protocol.sanitizeOutput(std.testing.allocator, "failed: very-secret", &secrets);
    defer std.testing.allocator.free(clean);
    try std.testing.expectEqualStrings("failed: [REDACTED]", clean);
}

test "operation payload types reject duplicate and unknown fields" {
    var valid = try protocol.parseStrict(protocol.InspectSourcePayload, std.testing.allocator,
        \\{"source_path":"app.json","schema_path":"app.schema.json"}
    );
    valid.deinit();
    try std.testing.expectError(error.UnknownField, protocol.parseStrict(protocol.InspectSourcePayload, std.testing.allocator,
        \\{"source_path":"app.json","extra":true}
    ));
    try std.testing.expectError(error.DuplicateField, protocol.parseStrict(protocol.ApplyPayload, std.testing.allocator,
        \\{"source_path":"a","source_path":"b","source_digest":"sha256:a","final_change_digest":"sha256:b","confirmation_nonce":"n"}
    ));
}

test "protocol_info reports capabilities in a matching response envelope" {
    var parsed = try protocol.parseRequest(std.testing.allocator,
        \\{"protocol_version":"1.0","request_id":"hello","operation":"protocol_info","payload_schema":"zconfig.empty/1","payload":{}}
    );
    defer parsed.deinit();
    var output: std.Io.Writer.Allocating = .init(std.testing.allocator);
    defer output.deinit();
    try protocol.writeResponse(parsed.value, &output.writer);
    var response = try std.json.parseFromSlice(std.json.Value, std.testing.allocator, output.written(), .{});
    defer response.deinit();
    const object = response.value.object;
    try std.testing.expectEqualStrings("hello", object.get("request_id").?.string);
    try std.testing.expect(object.get("ok").?.bool);
    try std.testing.expectEqual(@as(i64, protocol.max_message_bytes), object.get("result").?.object.get("limits").?.object.get("message_bytes").?.integer);
}
