const std = @import("std");
const redact = @import("redact");

test "normalized conservative sensitive names" {
    try std.testing.expect(redact.isSensitiveName("/auth/api_key"));
    try std.testing.expect(redact.isSensitiveName("/auth/api_token"));
    try std.testing.expect(redact.isSensitiveName("/clientSecret"));
    try std.testing.expect(!redact.isSensitiveName("/tokenizer"));
    try std.testing.expectEqual(redact.Sensitivity.schema_sensitive, redact.classify("/normal", true));
    try std.testing.expectEqual(redact.Sensitivity.suspected_sensitive, redact.classify("/password", false));
}

test "reveal and share authorization are scoped fingerprint bound and expiring" {
    var share = redact.Authorization{ .change_id = "secret", .fingerprint_value = "fp", .capability = .share_once, .expires_at_ms = 200 };
    try std.testing.expectError(error.AuthorizationMismatch, share.validate(100, "other", "fp"));
    try std.testing.expectError(error.AuthorizationExpired, share.validate(201, "secret", "fp"));
    try share.validate(100, "secret", "fp");
    try std.testing.expectError(error.AuthorizationConsumed, share.validate(100, "secret", "fp"));
    var reveal = redact.Authorization{ .change_id = "secret", .fingerprint_value = "fp", .capability = .reveal, .expires_at_ms = 200 };
    try reveal.validate(100, "secret", "fp");
    try reveal.validate(100, "secret", "fp");
}

test "fingerprints are session bound and contain no value" {
    const a = redact.fingerprint("hunter2", "session-a");
    const b = redact.fingerprint("hunter2", "session-b");
    try std.testing.expect(!std.mem.eql(u8, &a, &b));
    try std.testing.expect(std.mem.startsWith(u8, &a, "hmac-sha256:"));
}
