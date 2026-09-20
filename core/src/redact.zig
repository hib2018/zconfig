const std = @import("std");
const protocol = @import("protocol.zig");

pub const Sensitivity = enum { normal, schema_sensitive, suspected_sensitive };

pub fn isSensitiveName(path: []const u8) bool {
    const slash = std.mem.lastIndexOfScalar(u8, path, '/') orelse 0;
    const raw = if (slash < path.len and path[slash] == '/') path[slash + 1 ..] else path;
    var normalized_buf: [256]u8 = undefined;
    if (raw.len > normalized_buf.len) return false;
    var len: usize = 0;
    for (raw) |c| {
        if (std.ascii.isAlphanumeric(c)) {
            normalized_buf[len] = std.ascii.toLower(c);
            len += 1;
        }
    }
    const normalized = normalized_buf[0..len];
    const names = [_][]const u8{
        "password", "passwd",   "secret",     "token",        "accesstoken", "refreshtoken",
        "apikey",   "apitoken", "privatekey", "clientsecret", "credential",  "credentials",
    };
    for (names) |name| if (std.mem.eql(u8, normalized, name)) return true;
    return false;
}

pub fn classify(path: []const u8, schema_sensitive: bool) Sensitivity {
    if (schema_sensitive) return .schema_sensitive;
    if (isSensitiveName(path)) return .suspected_sensitive;
    return .normal;
}

pub fn valueType(value: std.json.Value) protocol.ValueType {
    return switch (value) {
        .string => .string,
        .float => .number,
        .integer => .integer,
        .bool => .boolean,
        .null => .null,
        .array => .array,
        .object => .object,
        .number_string => .number,
    };
}

pub fn fingerprint(value_bytes: []const u8, session_key: []const u8) [76]u8 {
    const Hmac = std.crypto.auth.hmac.sha2.HmacSha256;
    var mac: [Hmac.mac_length]u8 = undefined;
    Hmac.create(&mac, value_bytes, session_key);
    var result: [76]u8 = undefined;
    @memcpy(result[0..12], "hmac-sha256:");
    const hex = std.fmt.bytesToHex(mac, .lower);
    @memcpy(result[12..], &hex);
    return result;
}

pub const Authorization = struct {
    change_id: []const u8,
    fingerprint_value: []const u8,
    capability: enum { reveal, share_once },
    expires_at_ms: i64,
    consumed: bool = false,

    pub fn validate(self: *Authorization, now_ms: i64, change_id: []const u8, expected_fingerprint: []const u8) !void {
        if (self.consumed) return error.AuthorizationConsumed;
        if (now_ms > self.expires_at_ms) return error.AuthorizationExpired;
        if (!std.mem.eql(u8, self.change_id, change_id) or !std.mem.eql(u8, self.fingerprint_value, expected_fingerprint)) return error.AuthorizationMismatch;
        if (self.capability == .share_once) self.consumed = true;
    }
};
