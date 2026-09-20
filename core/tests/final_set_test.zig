const std = @import("std");
const final_set = @import("final_set");
const protocol = final_set.protocol;

const source = "{\"a\":1,\"b\":2}";

fn payload(allocator: std.mem.Allocator, a: []const u8, b: []const u8, comments: []const protocol.FinalComment, checks: []const std.json.Value) !struct { value: protocol.FinalAssemblyPayload, decisions: std.json.ObjectMap } {
    var decisions: std.json.ObjectMap = .empty;
    try decisions.put(allocator, "a", .{ .string = a });
    try decisions.put(allocator, "b", .{ .string = b });
    const digest = final_set.digest(source);
    const items = try allocator.dupe(protocol.ChangeItem, &.{
        .{ .change_id = "a", .path = "/a", .operation = .replace, .expected_old = .{ .integer = 1 }, .proposed_value = .{ .integer = 10 }, .explanation = "a" },
        .{ .change_id = "b", .path = "/b", .operation = .replace, .expected_old = .{ .integer = 2 }, .proposed_value = .{ .integer = 20 }, .explanation = "b" },
    });
    return .{ .decisions = decisions, .value = .{
        .source_path = "app.json",
        .source_digest = try allocator.dupe(u8, &digest),
        .proposal = .{ .proposal_id = "p", .revision = 1, .source_digest = try allocator.dupe(u8, &digest), .created_at = "2026-09-20T00:00:00Z", .items = items },
        .decisions = .{ .object = decisions },
        .comments = comments,
        .external_checks = checks,
    } };
}

fn freePayload(allocator: std.mem.Allocator, p: anytype) void {
    allocator.free(p.value.source_digest);
    allocator.free(p.value.proposal.source_digest);
    allocator.free(p.value.proposal.items);
    var decisions = p.decisions;
    decisions.deinit(allocator);
}

test "approved subset is assembled while rejected item is excluded" {
    const p = try payload(std.testing.allocator, "approved", "rejected", &.{}, &.{});
    defer freePayload(std.testing.allocator, p);
    var result = try final_set.assemble(std.testing.allocator, source, p.value);
    defer result.deinit();
    try std.testing.expectEqualStrings("{\"a\":10,\"b\":2}", result.candidate);
    try std.testing.expectEqual(@as(usize, 1), result.approved_ids.len);
    try std.testing.expectEqualStrings("a", result.approved_ids[0]);
}

test "final diff preserves normal bytes and redacts schema and name sensitive values" {
    const allocator = std.testing.allocator;
    const diff_source = "{\"theme\":\"light\",\"token\":\"old-token\",\"credential\":\"old-credential\"}";
    const diff_digest = final_set.digest(diff_source);
    const items = [_]protocol.ChangeItem{
        .{ .change_id = "theme", .path = "/theme", .operation = .replace, .expected_old = .{ .string = "light" }, .proposed_value = .{ .string = "dark" }, .explanation = "theme" },
        .{ .change_id = "token", .path = "/token", .operation = .replace, .expected_old = .{ .string = "old-token" }, .proposed_value = .{ .string = "new-token" }, .explanation = "token" },
        .{ .change_id = "credential", .path = "/credential", .operation = .replace, .expected_old = .{ .string = "old-credential" }, .proposed_value = .{ .string = "new-credential" }, .explanation = "credential" },
    };
    var decisions: std.json.ObjectMap = .empty;
    defer decisions.deinit(allocator);
    inline for (.{ "theme", "token", "credential" }) |id| try decisions.put(allocator, id, .{ .string = "approved" });
    const value = protocol.FinalAssemblyPayload{
        .source_path = "app.json",
        .source_digest = &diff_digest,
        .proposal = .{ .proposal_id = "p", .revision = 1, .source_digest = &diff_digest, .created_at = "2026-09-20T00:00:00Z", .items = &items },
        .decisions = .{ .object = decisions },
        .comments = &.{},
        .external_checks = &.{},
    };
    var assembled = try final_set.assemble(allocator, diff_source, value);
    defer assembled.deinit();
    var parsed_schema = try std.json.parseFromSlice(std.json.Value, allocator,
        \\{"type":"object","properties":{"credential":{"type":"string","x-zconfig-sensitive":true}}}
    , .{});
    defer parsed_schema.deinit();
    const diff = try final_set.renderDiff(allocator, diff_source, assembled.candidate, value.proposal, assembled.approved_ids, parsed_schema.value);
    defer allocator.free(diff);
    try std.testing.expect(std.mem.indexOf(u8, diff, "- \"light\"\n+ \"dark\"") != null);
    try std.testing.expect(std.mem.indexOf(u8, diff, "old-token") == null);
    try std.testing.expect(std.mem.indexOf(u8, diff, "new-token") == null);
    try std.testing.expect(std.mem.indexOf(u8, diff, "old-credential") == null);
    try std.testing.expect(std.mem.indexOf(u8, diff, "new-credential") == null);
    try std.testing.expectEqual(@as(usize, 4), std.mem.count(u8, diff, "[REDACTED]"));
}

test "pending decisions unresolved comments and failed checks block assembly" {
    const pending = try payload(std.testing.allocator, "pending", "rejected", &.{}, &.{});
    defer freePayload(std.testing.allocator, pending);
    try std.testing.expectError(error.PendingDecision, final_set.assemble(std.testing.allocator, source, pending.value));

    const comments = [_]protocol.FinalComment{.{ .comment_id = "c", .change_id = "a", .status = .agent_claimed }};
    const unresolved = try payload(std.testing.allocator, "approved", "rejected", &comments, &.{});
    defer freePayload(std.testing.allocator, unresolved);
    try std.testing.expectError(error.UnresolvedComment, final_set.assemble(std.testing.allocator, source, unresolved.value));

    var check_object: std.json.ObjectMap = .empty;
    defer check_object.deinit(std.testing.allocator);
    try check_object.put(std.testing.allocator, "status", .{ .string = "failed" });
    var check_array = std.json.Array.init(std.testing.allocator);
    defer check_array.deinit();
    try check_array.append(.{ .object = check_object });
    var set_object: std.json.ObjectMap = .empty;
    defer set_object.deinit(std.testing.allocator);
    const expected_digest = final_set.digest("{\"a\":10,\"b\":2}");
    try set_object.put(std.testing.allocator, "subject_digest", .{ .string = &expected_digest });
    try set_object.put(std.testing.allocator, "checks", .{ .array = check_array });
    const checks = [_]std.json.Value{.{ .object = set_object }};
    const failed = try payload(std.testing.allocator, "approved", "rejected", &.{}, &checks);
    defer freePayload(std.testing.allocator, failed);
    try std.testing.expectError(error.MandatoryCheckFailed, final_set.assemble(std.testing.allocator, source, failed.value));

    var wrong_set: std.json.ObjectMap = .empty;
    defer wrong_set.deinit(std.testing.allocator);
    try wrong_set.put(std.testing.allocator, "subject_digest", .{ .string = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" });
    try wrong_set.put(std.testing.allocator, "checks", .{ .array = check_array });
    const wrong_checks = [_]std.json.Value{.{ .object = wrong_set }};
    const mismatched = try payload(std.testing.allocator, "approved", "rejected", &.{}, &wrong_checks);
    defer freePayload(std.testing.allocator, mismatched);
    try std.testing.expectError(error.CheckDigestMismatch, final_set.assemble(std.testing.allocator, source, mismatched.value));
}

test "capability is digest bound expiring and one use" {
    var capability = final_set.Capability{ .nonce = "0123456789abcdef0123456789abcdef", .source_digest = "source", .final_change_digest = "final", .issued_at_ms = 100, .expires_at_ms = 200 };
    try std.testing.expectError(error.ConfirmationDigestMismatch, capability.authorize(150, capability.nonce, "source", "other"));
    try std.testing.expectError(error.ConfirmationExpired, capability.authorize(201, capability.nonce, "source", "final"));
    try capability.authorize(150, capability.nonce, "source", "final");
    try std.testing.expectError(error.ConfirmationConsumed, capability.authorize(150, capability.nonce, "source", "final"));
}
