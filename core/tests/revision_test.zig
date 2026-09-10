const std = @import("std");
const proposal = @import("proposal");

const active_json =
    \\{"proposal_id":"p","revision":1,"source_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","created_at":"2026-09-11T00:00:00Z","items":[{"change_id":"a","path":"/a","operation":"replace","expected_old":1,"proposed_value":2,"explanation":"a"},{"change_id":"b","path":"/b","operation":"replace","expected_old":3,"proposed_value":4,"explanation":"b"}]}
;

test "revision changes only allowed proposed values" {
    var active = try proposal.parse(std.testing.allocator, active_json);
    defer active.deinit();
    var candidate = try proposal.parse(std.testing.allocator,
        \\{"proposal_id":"p","revision":2,"source_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","created_at":"2026-09-11T00:01:00Z","items":[{"change_id":"a","path":"/a","operation":"replace","expected_old":1,"proposed_value":9,"explanation":"revised"},{"change_id":"b","path":"/b","operation":"replace","expected_old":3,"proposed_value":4,"explanation":"b"}]}
    );
    defer candidate.deinit();
    try proposal.validateRevision(active.value, candidate.value, &.{"a"});
}

test "out of scope or immutable changes reject the whole revision" {
    var active = try proposal.parse(std.testing.allocator, active_json);
    defer active.deinit();
    const invalid = [_][]const u8{
        \\{"proposal_id":"p","revision":2,"source_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","created_at":"2026-09-11T00:01:00Z","items":[{"change_id":"a","path":"/a","operation":"replace","expected_old":1,"proposed_value":2,"explanation":"a"},{"change_id":"b","path":"/b","operation":"replace","expected_old":3,"proposed_value":8,"explanation":"b"}]}
        ,
        \\{"proposal_id":"p","revision":2,"source_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","created_at":"2026-09-11T00:01:00Z","items":[{"change_id":"a","path":"/changed","operation":"replace","expected_old":1,"proposed_value":9,"explanation":"a"},{"change_id":"b","path":"/b","operation":"replace","expected_old":3,"proposed_value":4,"explanation":"b"}]}
        ,
    };
    for (invalid) |input| {
        var candidate = try proposal.parse(std.testing.allocator, input);
        defer candidate.deinit();
        try std.testing.expectError(error.ScopeViolation, proposal.validateRevision(active.value, candidate.value, &.{"a"}));
    }
}
