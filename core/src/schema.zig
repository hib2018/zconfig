const std = @import("std");

pub const Status = enum { passed, failed, unverified };
pub const Result = struct {
    status: Status,
    code: []const u8,
    unsupported_keyword: ?[]const u8 = null,
};

pub const ParsedSchema = struct {
    parsed: std.json.Parsed(std.json.Value),

    pub fn deinit(self: *ParsedSchema) void {
        self.parsed.deinit();
    }
};

pub fn parse(allocator: std.mem.Allocator, bytes: []const u8) !ParsedSchema {
    var parsed = std.json.parseFromSlice(std.json.Value, allocator, bytes, .{}) catch |err| return switch (err) {
        error.DuplicateField => error.DuplicateField,
        else => error.InvalidSchema,
    };
    errdefer parsed.deinit();
    if (parsed.value != .object and parsed.value != .bool) return error.InvalidSchema;
    return .{ .parsed = parsed };
}

pub fn validate(value: std.json.Value, schema_value: std.json.Value) Result {
    return validateInner(value, schema_value);
}

fn validateInner(value: std.json.Value, schema_value: std.json.Value) Result {
    if (schema_value == .bool) return if (schema_value.bool)
        .{ .status = .passed, .code = "schema.valid" }
    else
        .{ .status = .failed, .code = "schema.false" };
    const object = switch (schema_value) {
        .object => |v| v,
        else => return .{ .status = .failed, .code = "schema.invalid" },
    };

    var iterator = object.iterator();
    while (iterator.next()) |entry| {
        if (!isSupported(entry.key_ptr.*)) return .{
            .status = .unverified,
            .code = "schema.unsupported_keyword",
            .unsupported_keyword = entry.key_ptr.*,
        };
    }
    if (object.get("type")) |expected| if (!matchesType(value, expected)) return .{ .status = .failed, .code = "schema.type" };
    if (object.get("const")) |expected| if (!valueEqual(value, expected)) return .{ .status = .failed, .code = "schema.const" };
    if (object.get("enum")) |choices| {
        const array = switch (choices) {
            .array => |v| v,
            else => return .{ .status = .failed, .code = "schema.invalid_enum" },
        };
        var found = false;
        for (array.items) |choice| if (valueEqual(value, choice)) {
            found = true;
            break;
        };
        if (!found) return .{ .status = .failed, .code = "schema.enum" };
    }
    if (numeric(value)) |number| {
        if (object.get("minimum")) |bound| if (numeric(bound)) |n| if (number < n) return .{ .status = .failed, .code = "schema.minimum" };
        if (object.get("maximum")) |bound| if (numeric(bound)) |n| if (number > n) return .{ .status = .failed, .code = "schema.maximum" };
        if (object.get("exclusiveMinimum")) |bound| if (numeric(bound)) |n| if (number <= n) return .{ .status = .failed, .code = "schema.exclusive_minimum" };
        if (object.get("exclusiveMaximum")) |bound| if (numeric(bound)) |n| if (number >= n) return .{ .status = .failed, .code = "schema.exclusive_maximum" };
    }
    if (value == .string) {
        const len = std.unicode.utf8CountCodepoints(value.string) catch return .{ .status = .failed, .code = "schema.invalid_utf8" };
        if (object.get("minLength")) |bound| if (bound == .integer and len < @as(usize, @intCast(bound.integer))) return .{ .status = .failed, .code = "schema.min_length" };
        if (object.get("maxLength")) |bound| if (bound == .integer and len > @as(usize, @intCast(bound.integer))) return .{ .status = .failed, .code = "schema.max_length" };
        if (object.get("pattern")) |pattern| if (pattern == .string and !matchesSimplePattern(value.string, pattern.string)) return .{ .status = .failed, .code = "schema.pattern" };
    }
    if (value == .array) {
        if (object.get("minItems")) |bound| if (bound == .integer and value.array.items.len < @as(usize, @intCast(bound.integer))) return .{ .status = .failed, .code = "schema.min_items" };
        if (object.get("maxItems")) |bound| if (bound == .integer and value.array.items.len > @as(usize, @intCast(bound.integer))) return .{ .status = .failed, .code = "schema.max_items" };
        if (object.get("items")) |item_schema| for (value.array.items) |item| {
            const result = validateInner(item, item_schema);
            if (result.status != .passed) return result;
        };
    }
    if (value == .object) {
        if (object.get("required")) |required| if (required == .array) for (required.array.items) |name| {
            if (name == .string and !value.object.contains(name.string)) return .{ .status = .failed, .code = "schema.required" };
        };
        if (object.get("properties")) |properties| if (properties == .object) {
            var properties_it = properties.object.iterator();
            while (properties_it.next()) |entry| if (value.object.get(entry.key_ptr.*)) |child| {
                const result = validateInner(child, entry.value_ptr.*);
                if (result.status != .passed) return result;
            };
        };
    }
    return .{ .status = .passed, .code = "schema.valid" };
}

pub fn isSensitive(schema_value: std.json.Value) bool {
    if (schema_value != .object) return false;
    const annotation = schema_value.object.get("x-zconfig-sensitive") orelse return false;
    return annotation == .bool and annotation.bool;
}

fn isSupported(key: []const u8) bool {
    const supported = [_][]const u8{
        "$schema",             "$id",     "type",             "properties",       "required",  "items",      "enum",     "const",
        "minimum",             "maximum", "exclusiveMinimum", "exclusiveMaximum", "minLength", "maxLength",  "pattern",  "minItems",
        "maxItems",            "title",   "description",      "default",          "examples",  "deprecated", "readOnly", "writeOnly",
        "x-zconfig-sensitive",
    };
    for (supported) |candidate| if (std.mem.eql(u8, key, candidate)) return true;
    return false;
}

fn matchesType(value: std.json.Value, expected: std.json.Value) bool {
    if (expected == .string) return oneType(value, expected.string);
    if (expected == .array) for (expected.array.items) |item| if (item == .string and oneType(value, item.string)) return true;
    return false;
}

fn oneType(value: std.json.Value, name: []const u8) bool {
    if (std.mem.eql(u8, name, "null")) return value == .null;
    if (std.mem.eql(u8, name, "boolean")) return value == .bool;
    if (std.mem.eql(u8, name, "object")) return value == .object;
    if (std.mem.eql(u8, name, "array")) return value == .array;
    if (std.mem.eql(u8, name, "string")) return value == .string;
    if (std.mem.eql(u8, name, "integer")) return value == .integer;
    if (std.mem.eql(u8, name, "number")) return value == .integer or value == .float or value == .number_string;
    return false;
}

fn numeric(value: std.json.Value) ?f64 {
    return switch (value) {
        .integer => |v| @floatFromInt(v),
        .float => |v| v,
        .number_string => |v| std.fmt.parseFloat(f64, v) catch null,
        else => null,
    };
}

fn matchesSimplePattern(value: []const u8, pattern: []const u8) bool {
    if (std.mem.eql(u8, pattern, "^[a-z]+$")) {
        if (value.len == 0) return false;
        for (value) |c| if (c < 'a' or c > 'z') return false;
        return true;
    }
    if (std.mem.eql(u8, pattern, "^[0-9]+$")) {
        if (value.len == 0) return false;
        for (value) |c| if (!std.ascii.isDigit(c)) return false;
        return true;
    }
    if (pattern.len >= 2 and pattern[0] == '^' and pattern[pattern.len - 1] == '$') return std.mem.eql(u8, value, pattern[1 .. pattern.len - 1]);
    return std.mem.indexOf(u8, value, pattern) != null;
}

fn valueEqual(a: std.json.Value, b: std.json.Value) bool {
    if (@intFromEnum(a) != @intFromEnum(b)) return numeric(a) != null and numeric(b) != null and numeric(a).? == numeric(b).?;
    return switch (a) {
        .null => true,
        .bool => |v| v == b.bool,
        .integer => |v| v == b.integer,
        .float => |v| v == b.float,
        .number_string => |v| std.mem.eql(u8, v, b.number_string),
        .string => |v| std.mem.eql(u8, v, b.string),
        .array => |v| blk: {
            if (v.items.len != b.array.items.len) break :blk false;
            for (v.items, b.array.items) |x, y| if (!valueEqual(x, y)) break :blk false;
            break :blk true;
        },
        .object => |v| blk: {
            if (v.count() != b.object.count()) break :blk false;
            var it = v.iterator();
            while (it.next()) |entry| {
                const other = b.object.get(entry.key_ptr.*) orelse break :blk false;
                if (!valueEqual(entry.value_ptr.*, other)) break :blk false;
            }
            break :blk true;
        },
    };
}
