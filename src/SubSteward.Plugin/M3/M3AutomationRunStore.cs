using System;

namespace SubSteward.Plugin.M3
{
    /// <summary>
    /// Holds only the latest compact run summary until a real persistence
    /// requirement exists. The store is process-local and is not a history or
    /// recovery system.
    /// </summary>
    public static class M3AutomationRunStore
    {
        private static readonly object Sync = new object();
        private static M3AutomationRunSummary latest;

        public static void Record(M3AutomationRunResult result)
        {
            if (result == null)
            {
                return;
            }

            lock (Sync)
            {
                latest = ToSummary(result);
            }
        }

        public static M3AutomationRunSummary GetLatest()
        {
            lock (Sync)
            {
                return latest == null ? null : ToSummary(latest);
            }
        }

        private static M3AutomationRunSummary ToSummary(M3AutomationRunResult result)
        {
            return new M3AutomationRunSummary
            {
                StartedAtUtc = result.StartedAtUtc,
                CompletedAtUtc = result.CompletedAtUtc,
                DryRun = result.DryRun,
                Status = M3AutomationResultLabels.ForCode(result.Status),
                StatusCode = result.Status,
                LibraryCount = result.LibraryCount,
                ScannedCount = result.ScannedCount,
                CompletedCount = result.CompletedCount,
                SkippedCount = result.SkippedCount,
                FailedCount = result.FailedCount,
                ManualCount = result.ManualCount,
                SynchronizationPassCount = result.SynchronizationPassCount,
                SynchronizationUnknownCount = result.SynchronizationUnknownCount,
                SynchronizationDriftCount = result.SynchronizationDriftCount,
                LastSynchronizationMethod = result.LastSynchronizationMethod,
                LastTimelineOffsetMilliseconds = result.LastTimelineOffsetMilliseconds,
                LastSynchronizationReason = result.LastSynchronizationReason
            };
        }

        private static M3AutomationRunSummary ToSummary(M3AutomationRunSummary summary)
        {
            return new M3AutomationRunSummary
            {
                StartedAtUtc = summary.StartedAtUtc,
                CompletedAtUtc = summary.CompletedAtUtc,
                DryRun = summary.DryRun,
                Status = summary.Status,
                StatusCode = summary.StatusCode,
                LibraryCount = summary.LibraryCount,
                ScannedCount = summary.ScannedCount,
                CompletedCount = summary.CompletedCount,
                SkippedCount = summary.SkippedCount,
                FailedCount = summary.FailedCount,
                ManualCount = summary.ManualCount,
                SynchronizationPassCount = summary.SynchronizationPassCount,
                SynchronizationUnknownCount = summary.SynchronizationUnknownCount,
                SynchronizationDriftCount = summary.SynchronizationDriftCount,
                LastSynchronizationMethod = summary.LastSynchronizationMethod,
                LastTimelineOffsetMilliseconds = summary.LastTimelineOffsetMilliseconds,
                LastSynchronizationReason = summary.LastSynchronizationReason
            };
        }
    }
}
