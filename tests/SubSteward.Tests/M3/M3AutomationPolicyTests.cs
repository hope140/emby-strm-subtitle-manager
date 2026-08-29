using SubSteward.Plugin.M3;
using Xunit;

namespace SubSteward.Tests;

public sealed class M3AutomationPolicyTests
{
    private readonly M3AutomationPolicy policy = new M3AutomationPolicy();

    [Fact]
    public void DisabledAutomation_SkipsWithoutInspectingSources()
    {
        var report = policy.EvaluateItem(new M3EligibilityInput
        {
            AutomationEnabled = false,
            LibraryAuthorized = true,
            SourceCount = 1,
            TargetLanguagePresent = false,
            HasSourceIdentity = true,
            HasSafeWriteAnchor = true
        });

        Assert.False(report.IsEligible);
        Assert.Equal(M3AutomationDecisionNames.Skip, report.Decision);
    }

    [Fact]
    public void EmptyAuthorization_IsNotEligible()
    {
        var report = policy.EvaluateItem(new M3EligibilityInput
        {
            AutomationEnabled = true,
            LibraryAuthorized = false,
            SourceCount = 1,
            TargetLanguagePresent = false,
            HasSourceIdentity = true,
            HasSafeWriteAnchor = true
        });

        Assert.False(report.IsEligible);
        Assert.Contains(report.Reasons, reason => reason.Contains("allowlist"));
    }

    [Fact]
    public void EligibleItem_RequiresMissingTargetAndSafeSingleSource()
    {
        var report = policy.EvaluateItem(new M3EligibilityInput
        {
            AutomationEnabled = true,
            LibraryAuthorized = true,
            SourceCount = 1,
            TargetLanguagePresent = false,
            HasSourceIdentity = true,
            HasSafeWriteAnchor = true
        });

        Assert.True(report.IsEligible);
        Assert.Equal(M3AutomationDecisionNames.Eligible, report.Decision);
    }

    [Fact]
    public void PresentTarget_IsSkippedAndNeverEligibleForAutomaticReplacement()
    {
        var report = policy.EvaluateItem(new M3EligibilityInput
        {
            AutomationEnabled = true,
            LibraryAuthorized = true,
            SourceCount = 1,
            TargetLanguagePresent = true,
            HasSourceIdentity = true,
            HasSafeWriteAnchor = true
        });

        Assert.False(report.IsEligible);
        Assert.Equal(M3AutomationDecisionNames.Skip, report.Decision);
    }

    [Fact]
    public void MultipleSources_RequireManualHandling()
    {
        var report = policy.EvaluateItem(new M3EligibilityInput
        {
            AutomationEnabled = true,
            LibraryAuthorized = true,
            SourceCount = 2,
            TargetLanguagePresent = false,
            HasSourceIdentity = true,
            HasSafeWriteAnchor = true
        });

        Assert.False(report.IsEligible);
        Assert.Equal(M3AutomationDecisionNames.Manual, report.Decision);
    }

    [Fact]
    public void UnsafeWriteAnchor_RequiresManualHandling()
    {
        var report = policy.EvaluateItem(new M3EligibilityInput
        {
            AutomationEnabled = true,
            LibraryAuthorized = true,
            SourceCount = 1,
            TargetLanguagePresent = false,
            HasSourceIdentity = true,
            HasSafeWriteAnchor = false
        });

        Assert.False(report.IsEligible);
        Assert.Equal(M3AutomationDecisionNames.Manual, report.Decision);
        Assert.Contains(report.Reasons, reason => reason.Contains("safe local"));
    }

    [Fact]
    public void TitleOnlyCandidate_IsRejectedForAutomaticFetch()
    {
        var report = policy.EvaluateCandidateMetadata(new M3CandidateMetadataInput
        {
            TitleMatch = true,
            HashMatch = false
        });

        Assert.False(report.IsEligible);
        Assert.Equal(M3AutomationDecisionNames.Reject, report.Decision);
        Assert.Equal(70, report.MatchScore);
    }

    [Fact]
    public void StrmTitleAndYearCandidate_IsEligibleWithoutHash()
    {
        var report = policy.EvaluateCandidateMetadata(new M3CandidateMetadataInput
        {
            IsStrm = true,
            TitleMatch = true,
            ReleaseYearMatch = true,
            HashMatch = false
        });

        Assert.True(report.IsEligible);
        Assert.Equal(M3AutomationDecisionNames.Eligible, report.Decision);
        Assert.Equal(85, report.MatchScore);
    }

    [Fact]
    public void StrmHashAlone_IsNotAnAutomaticMatch()
    {
        var report = policy.EvaluateCandidateMetadata(new M3CandidateMetadataInput
        {
            IsStrm = true,
            HashMatch = true
        });

        Assert.False(report.IsEligible);
        Assert.Equal(M3AutomationDecisionNames.Reject, report.Decision);
        Assert.Contains(report.Reasons, reason => reason.Contains("title and year/episode"));
    }

    [Fact]
    public void HashMatchedRecommendedPassCandidate_IsEligible()
    {
        var report = policy.EvaluateCandidate(new M3CandidateGateInput
        {
            HashMatch = true,
            TitleMatch = false,
            Health = "PASS",
            TargetLanguagePresent = true,
            PreferenceSuitability = "RECOMMENDED"
        });

        Assert.True(report.IsEligible);
        Assert.Equal(M3AutomationDecisionNames.Eligible, report.Decision);
        Assert.Equal(100, report.MatchScore);
    }

    [Fact]
    public void StrmTitleAndYearRecommendedPassCandidate_IsEligibleWithoutHash()
    {
        var report = policy.EvaluateCandidate(new M3CandidateGateInput
        {
            IsStrm = true,
            TitleMatch = true,
            ReleaseYearMatch = true,
            Health = "PASS",
            TargetLanguagePresent = true,
            PreferenceSuitability = "RECOMMENDED"
        });

        Assert.True(report.IsEligible);
        Assert.Equal(M3AutomationDecisionNames.Eligible, report.Decision);
        Assert.Contains(report.Reasons, reason => reason.Contains("STRM matching"));
    }

    [Fact]
    public void WarningCandidate_RequiresManualReview()
    {
        var report = policy.EvaluateCandidate(new M3CandidateGateInput
        {
            HashMatch = true,
            Health = "WARNING",
            TargetLanguagePresent = true,
            PreferenceSuitability = "RECOMMENDED"
        });

        Assert.False(report.IsEligible);
        Assert.Equal(M3AutomationDecisionNames.Manual, report.Decision);
    }

    [Fact]
    public void AcceptableCandidate_IsNotInstalledAutomatically()
    {
        var report = policy.EvaluateCandidate(new M3CandidateGateInput
        {
            HashMatch = true,
            Health = "PASS",
            TargetLanguagePresent = true,
            PreferenceSuitability = "ACCEPTABLE"
        });

        Assert.False(report.IsEligible);
        Assert.Equal(M3AutomationDecisionNames.Manual, report.Decision);
    }

    [Fact]
    public void LowConfidenceBilingualCandidate_DoesNotBlockMissingTargetSupplement()
    {
        var report = policy.EvaluateCandidate(new M3CandidateGateInput
        {
            HashMatch = true,
            Health = "PASS",
            TargetLanguagePresent = true,
            PreferenceSuitability = "RECOMMENDED",
            PreferBilingual = true,
            BilingualDetected = true,
            BilingualConfidence = 0.2d
        });

        Assert.True(report.IsEligible);
        Assert.Equal(M3AutomationDecisionNames.Eligible, report.Decision);
    }

    [Fact]
    public void Task_IsRegisteredWithSingleDailyTriggerAndOneConstructor()
    {
        Assert.Single(typeof(M3SubtitleAutomationTask).GetConstructors());
        Assert.True(typeof(MediaBrowser.Model.Tasks.IScheduledTask).IsAssignableFrom(typeof(M3SubtitleAutomationTask)));
        Assert.Equal(System.TimeSpan.FromHours(24), M3SubtitleAutomationTask.DefaultInterval);
    }

    [Fact]
    public void ResultLabels_AreChineseForUserFacingStatus()
    {
        Assert.Equal("已完成", M3AutomationResultLabels.ForCode(M3AutomationResultNames.Completed));
        Assert.Equal("已跳过", M3AutomationResultLabels.ForCode(M3AutomationResultNames.Skipped));
        Assert.Equal("失败", M3AutomationResultLabels.ForCode(M3AutomationResultNames.Failed));
        Assert.Equal("需人工", M3AutomationResultLabels.ForCode(M3AutomationResultNames.Manual));
    }
}
