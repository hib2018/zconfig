const std = @import("std");
const redact = @import("redact");

test "normalized conservative sensitive names" {
    try std.testing.expect(redact.isSensitiveName("/auth/api_key"));
    try std.testing.expect(redact.isSensitiveName("/clientSecret"));
    try std.testing.expect(!redact.isSensitiveName("/tokenizer"));
    try std.testing.expectEqual(redact.Sensitivity.schema_sensitive, redact.classify("/normal", true));
    try std.testing.expectEqual(redact.Sensitivity.suspected_sensitive, redact.classify("/password", false));
}

test "fingerprints are session bound and contain no value" {
    const a = redact.fingerprint("hunter2", "session-a");
    const b = redact.fingerprint("hunter2", "session-b");
    try std.testing.expect(!std.mem.eql(u8, &a, &b));
    try std.testing.expect(std.mem.startsWith(u8, &a, "hmac-sha256:"));
}
