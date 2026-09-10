const std = @import("std");
const proposal = @import("proposal");

const digest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";

test "proposal accepts add replace and remove" {
    const input =
        \\{"proposal_id":"p1","revision":1,"source_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","created_at":"2026-09-11T00:00:00Z","items":[{"change_id":"a","path":"/new","operation":"add","proposed_value":1,"explanation":"add"},{"change_id":"b","path":"/old","operation":"replace","expected_old":1,"proposed_value":2,"explanation":"replace"},{"change_id":"c","path":"/gone","operation":"remove","expected_old":true,"explanation":"remove"}]}
    ;
    var parsed = try proposal.parse(std.testing.allocator, input);
    defer parsed.deinit();
    try std.testing.expectEqualStrings(digest, parsed.value.source_digest);
}

test "expected old values and add targets are checked against source" {
    var source = try std.json.parseFromSlice(std.json.Value, std.testing.allocator, "{\"old\":1}", .{});
    defer source.deinit();
    const good =
        \\{"proposal_id":"p","revision":1,"source_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","created_at":"2026-09-11T00:00:00Z","items":[{"change_id":"a","path":"/old","operation":"replace","expected_old":1,"proposed_value":2,"explanation":"x"},{"change_id":"b","path":"/new","operation":"add","proposed_value":3,"explanation":"x"}]}
    ;
    var parsed = try proposal.parse(std.testing.allocator, good);
    defer parsed.deinit();
    try proposal.validateAgainstSource(std.testing.allocator, parsed.value, &source.value);

    const bad =
        \\{"proposal_id":"p","revision":1,"source_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","created_at":"2026-09-11T00:00:00Z","items":[{"change_id":"a","path":"/old","operation":"replace","expected_old":9,"proposed_value":2,"explanation":"x"}]}
    ;
    var mismatch = try proposal.parse(std.testing.allocator, bad);
    defer mismatch.deinit();
    try std.testing.expectError(error.ExpectedOldMismatch, proposal.validateAgainstSource(std.testing.allocator, mismatch.value, &source.value));
}

test "proposal rejects operation shape IDs digests pointers and overlaps" {
    const cases = [_][]const u8{
        \\{"proposal_id":"p","revision":1,"source_digest":"bad","created_at":"2026-09-11T00:00:00Z","items":[{"change_id":"a","path":"/a","operation":"add","proposed_value":1,"explanation":"x"}]}
        ,
        \\{"proposal_id":"p","revision":1,"source_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","created_at":"2026-09-11T00:00:00Z","items":[{"change_id":"a","path":"bad","operation":"add","proposed_value":1,"explanation":"x"}]}
        ,
        \\{"proposal_id":"p","revision":1,"source_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","created_at":"2026-09-11T00:00:00Z","items":[{"change_id":"a","path":"/a","operation":"add","expected_old":0,"proposed_value":1,"explanation":"x"}]}
        ,
        \\{"proposal_id":"p","revision":1,"source_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","created_at":"2026-09-11T00:00:00Z","items":[{"change_id":"a","path":"/a","operation":"add","proposed_value":1,"explanation":"x"},{"change_id":"b","path":"/a/b","operation":"add","proposed_value":2,"explanation":"x"}]}
        ,
    };
    for (cases) |input| {
        if (proposal.parse(std.testing.allocator, input)) |parsed_value| {
            var parsed = parsed_value;
            parsed.deinit();
            return error.TestExpectedError;
        } else |_| {}
    }
}
