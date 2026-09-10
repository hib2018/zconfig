const std = @import("std");

pub fn build(b: *std.Build) void {
    const target = b.standardTargetOptions(.{});
    const optimize = b.standardOptimizeOption(.{});

    const core_module = b.addModule("zconfig_core", .{
        .root_source_file = b.path("core/src/root.zig"),
        .target = target,
    });

    const exe = b.addExecutable(.{
        .name = "zconfig-core",
        .root_module = b.createModule(.{
            .root_source_file = b.path("core/src/main.zig"),
            .target = target,
            .optimize = optimize,
            .imports = &.{.{ .name = "zconfig_core", .module = core_module }},
        }),
    });
    b.installArtifact(exe);

    const run_cmd = b.addRunArtifact(exe);
    run_cmd.step.dependOn(b.getInstallStep());
    if (b.args) |args| run_cmd.addArgs(args);
    const run_step = b.step("run", "Run zconfig-core");
    run_step.dependOn(&run_cmd.step);

    const core_tests = b.addTest(.{
        .root_module = b.createModule(.{
            .root_source_file = b.path("core/src/root.zig"),
            .target = target,
            .optimize = optimize,
        }),
    });
    const run_core_tests = b.addRunArtifact(core_tests);
    const contract_tests = b.addTest(.{
        .root_module = b.createModule(.{
            .root_source_file = b.path("core/tests/contract.zig"),
            .target = target,
            .optimize = optimize,
            .imports = &.{.{ .name = "zconfig_core", .module = core_module }},
        }),
    });
    const run_contract_tests = b.addRunArtifact(contract_tests);
    const pointer_module = b.createModule(.{ .root_source_file = b.path("core/src/pointer.zig"), .target = target });
    const source_index_module = b.createModule(.{ .root_source_file = b.path("core/src/source_index.zig"), .target = target });
    const proposal_module = b.createModule(.{ .root_source_file = b.path("core/src/proposal.zig"), .target = target });
    const schema_module = b.createModule(.{ .root_source_file = b.path("core/src/schema.zig"), .target = target });
    const redaction_module = b.createModule(.{ .root_source_file = b.path("core/src/redact.zig"), .target = target });
    const document_module = b.createModule(.{ .root_source_file = b.path("core/src/document.zig"), .target = target });
    const us1_test_specs = [_]struct { path: []const u8, name: []const u8, module: *std.Build.Module }{
        .{ .path = "core/tests/pointer_test.zig", .name = "pointer", .module = pointer_module },
        .{ .path = "core/tests/source_index_test.zig", .name = "source_index", .module = source_index_module },
        .{ .path = "core/tests/proposal_test.zig", .name = "proposal", .module = proposal_module },
        .{ .path = "core/tests/schema_test.zig", .name = "schema", .module = schema_module },
        .{ .path = "core/tests/redaction_test.zig", .name = "redact", .module = redaction_module },
        .{ .path = "core/tests/document_test.zig", .name = "document", .module = document_module },
        .{ .path = "core/tests/revision_test.zig", .name = "proposal", .module = proposal_module },
    };
    const test_step = b.step("test", "Run core tests");
    test_step.dependOn(&run_core_tests.step);
    test_step.dependOn(&run_contract_tests.step);
    for (us1_test_specs) |spec| {
        const tests = b.addTest(.{
            .root_module = b.createModule(.{
                .root_source_file = b.path(spec.path),
                .target = target,
                .optimize = optimize,
                .imports = &.{.{ .name = spec.name, .module = spec.module }},
            }),
        });
        test_step.dependOn(&b.addRunArtifact(tests).step);
    }
}
