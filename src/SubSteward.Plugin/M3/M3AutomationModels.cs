using System;
using System.Collections.Generic;

namespace SubSteward.Plugin.M3
{
    public static class M3AutomationResultNames
    {
        public const string Completed = "COMPLETED";

        public const string Skipped = "SKIPPED";

        public const string Failed = "FAILED";

        public const string Manual = "MANUAL";
    }

    public static class M3AutomationResultLabels
    {
        public const string Completed = "已完成";

        public const string Skipped = "已跳过";

        public const string Failed = "失败";

        public const string Manual = "需人工";

        public static string ForCode(string code)
        {
            if (string.Equals(code, M3AutomationResultNames.Completed, StringComparison.OrdinalIgnoreCase))
            {
                return Completed;
            }

            if (string.Equals(code, M3AutomationResultNames.Failed, StringComparison.OrdinalIgnoreCase))
            {
                return Failed;
            }

            if (string.Equals(code, M3AutomationResultNames.Manual, StringComparison.OrdinalIgnoreCase))
            {
                return Manual;
            }

            return Skipped;
        }
    }

    public static class M3AutomationDecisionNames
    {
        public const string Eligible = "ELIGIBLE";

        public const string Skip = "SKIP";

        public const string Manual = "MANUAL";

        public const string Reject = "REJECT";
    }

    public sealed class M3EligibilityInput
    {
        public bool AutomationEnabled { get; set; }

        public bool LibraryAuthorized { get; set; }

        public bool StateKnown { get; set; } = true;

        public int SourceCount { get; set; }

        public bool? TargetLanguagePresent { get; set; }

        public bool HasSourceIdentity { get; set; }

        public bool HasSafeWriteAnchor { get; set; }

        public bool IsIgnored { get; set; }
    }

    public class M3CandidateMetadataInput
    {
        public bool HashMatch { get; set; }

        public bool TitleMatch { get; set; }

        public bool LikelyNonFullRelease { get; set; }

        public bool LanguageMismatch { get; set; }

        public bool VariantMismatch { get; set; }

        public bool IsStrm { get; set; }

        public bool ReleaseYearMatch { get; set; }

        public bool EpisodeMatch { get; set; }
    }

    public sealed class M3CandidateGateInput : M3CandidateMetadataInput
    {
        public string Health { get; set; }

        public bool TargetLanguagePresent { get; set; }

        public string PreferenceSuitability { get; set; }

        public bool PreferBilingual { get; set; }

        public bool BilingualDetected { get; set; }

        public double BilingualConfidence { get; set; }
    }

    public sealed class M3EligibilityReport
    {
        public bool IsEligible { get; set; }

        public string Decision { get; set; }

        public List<string> Reasons { get; } = new List<string>();
    }

    public sealed class M3CandidateGateReport
    {
        public bool IsEligible { get; set; }

        public string Decision { get; set; }

        public int MatchScore { get; set; }

        public List<string> Reasons { get; } = new List<string>();
    }

    public sealed class M3AutomationRunResult
    {
        public DateTime StartedAtUtc { get; set; }

        public DateTime CompletedAtUtc { get; set; }

        public bool DryRun { get; set; }

        public string Status { get; set; } = M3AutomationResultNames.Completed;

        public int LibraryCount { get; set; }

        public int ScannedCount { get; set; }

        public int CompletedCount { get; set; }

        public int SkippedCount { get; set; }

        public int FailedCount { get; set; }

        public int ManualCount { get; set; }

        public int SynchronizationPassCount { get; set; }

        public int SynchronizationUnknownCount { get; set; }

        public int SynchronizationDriftCount { get; set; }

        public string LastSynchronizationMethod { get; set; }

        public int LastTimelineOffsetMilliseconds { get; set; }

        public string LastSynchronizationReason { get; set; }

        public List<string> Reasons { get; } = new List<string>();

        public List<M3AutomationItemResult> Items { get; } = new List<M3AutomationItemResult>();
    }

    /// <summary>
    /// A safe, compact view of the latest run for the administrator API. It
    /// intentionally excludes item names, paths, provider IDs and subtitle
    /// content so the in-memory status store cannot become a history database.
    /// </summary>
    public sealed class M3AutomationRunSummary
    {
        public DateTime StartedAtUtc { get; set; }

        public DateTime CompletedAtUtc { get; set; }

        public bool DryRun { get; set; }

        public string Status { get; set; }

        public string StatusCode { get; set; }

        public int LibraryCount { get; set; }

        public int ScannedCount { get; set; }

        public int CompletedCount { get; set; }

        public int SkippedCount { get; set; }

        public int FailedCount { get; set; }

        public int ManualCount { get; set; }

        public int SynchronizationPassCount { get; set; }

        public int SynchronizationUnknownCount { get; set; }

        public int SynchronizationDriftCount { get; set; }

        public string LastSynchronizationMethod { get; set; }

        public int LastTimelineOffsetMilliseconds { get; set; }

        public string LastSynchronizationReason { get; set; }
    }

    public sealed class M3AutomationItemResult
    {
        public string ItemId { get; set; }

        public string ItemName { get; set; }

        public string LibraryName { get; set; }

        public string Status { get; set; }

        public string StatusLabel
        {
            get { return M3AutomationResultLabels.ForCode(Status); }
        }

        public int CandidateAttempts { get; set; }

        public string FileName { get; set; }

        public string SynchronizationStatus { get; set; }

        public string SynchronizationMethod { get; set; }

        public string SynchronizationReason { get; set; }

        public int TimelineOffsetMilliseconds { get; set; }

        public List<string> Reasons { get; } = new List<string>();
    }
}
