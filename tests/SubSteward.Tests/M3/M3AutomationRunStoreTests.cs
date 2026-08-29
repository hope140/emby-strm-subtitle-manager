using System;
using SubSteward.Plugin.M3;
using Xunit;

namespace SubSteward.Tests;

public sealed class M3AutomationRunStoreTests
{
    [Fact]
    public void LatestSummary_UsesChineseStatusAndKeepsMachineCode()
    {
        M3AutomationRunStore.Record(new M3AutomationRunResult
        {
            StartedAtUtc = DateTime.UtcNow,
            CompletedAtUtc = DateTime.UtcNow,
            Status = M3AutomationResultNames.Skipped,
            DryRun = true
        });

        var summary = M3AutomationRunStore.GetLatest();

        Assert.NotNull(summary);
        Assert.Equal("已跳过", summary.Status);
        Assert.Equal(M3AutomationResultNames.Skipped, summary.StatusCode);
    }

    [Fact]
    public void LatestSummary_ExposesSafeSynchronizationMetrics()
    {
        M3AutomationRunStore.Record(new M3AutomationRunResult
        {
            Status = M3AutomationResultNames.Completed,
            SynchronizationPassCount = 1,
            SynchronizationUnknownCount = 2,
            SynchronizationDriftCount = 3,
            LastSynchronizationMethod = "TEXT",
            LastTimelineOffsetMilliseconds = -2100
        });

        var summary = M3AutomationRunStore.GetLatest();

        Assert.Equal(1, summary.SynchronizationPassCount);
        Assert.Equal(2, summary.SynchronizationUnknownCount);
        Assert.Equal(3, summary.SynchronizationDriftCount);
        Assert.Equal("TEXT", summary.LastSynchronizationMethod);
        Assert.Equal(-2100, summary.LastTimelineOffsetMilliseconds);
    }
}
