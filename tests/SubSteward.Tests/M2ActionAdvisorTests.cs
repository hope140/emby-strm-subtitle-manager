using SubSteward.Plugin.M2;
using Xunit;

namespace SubSteward.Tests;

public sealed class M2ActionAdvisorTests
{
    [Fact]
    public void MissingTargetWithoutCandidate_RequestsSearch()
    {
        var report = Advise(new M2ActionInput
        {
            SourceCount = 1,
            TargetLanguagePresent = false
        });

        Assert.Equal(M2ActionNames.Search, report.Action);
    }

    [Fact]
    public void PresentHealthyTarget_IsKept()
    {
        var report = Advise(new M2ActionInput
        {
            SourceCount = 1,
            TargetLanguagePresent = true,
            ExistingTargetHealth = "PASS"
        });

        Assert.Equal(M2ActionNames.Keep, report.Action);
    }

    [Fact]
    public void MultipleSources_RequireManualHandling()
    {
        var report = Advise(new M2ActionInput
        {
            SourceCount = 2,
            TargetLanguagePresent = false,
            CandidateAvailable = true,
            CandidateHealth = "PASS",
            PreferenceSuitability = "RECOMMENDED",
            TitleMatch = true
        });

        Assert.Equal(M2ActionNames.Manual, report.Action);
        Assert.Contains(report.Reasons, reason => reason.Contains("exactly one MediaSource"));
    }

    [Fact]
    public void WarningCandidate_RequiresManualReview()
    {
        var report = Advise(new M2ActionInput
        {
            SourceCount = 1,
            TargetLanguagePresent = false,
            CandidateAvailable = true,
            CandidateHealth = "WARNING",
            PreferenceSuitability = "RECOMMENDED",
            TitleMatch = true
        });

        Assert.Equal(M2ActionNames.Manual, report.Action);
        Assert.Contains(report.Reasons, reason => reason.Contains("WARNING"));
    }

    [Fact]
    public void FailedCandidate_RequestsAnotherSearch()
    {
        var report = Advise(new M2ActionInput
        {
            SourceCount = 1,
            TargetLanguagePresent = false,
            CandidateAvailable = true,
            CandidateHealth = "FAIL",
            PreferenceSuitability = "RECOMMENDED",
            TitleMatch = true
        });

        Assert.Equal(M2ActionNames.Search, report.Action);
        Assert.Contains(report.Reasons, reason => reason.Contains("Health is FAIL"));
    }

    [Fact]
    public void UnboundCandidate_RequestsAnotherSearch()
    {
        var report = Advise(new M2ActionInput
        {
            SourceCount = 1,
            TargetLanguagePresent = false,
            CandidateAvailable = true,
            CandidateHealth = "PASS",
            PreferenceSuitability = "RECOMMENDED"
        });

        Assert.Equal(M2ActionNames.Search, report.Action);
        Assert.Contains(report.Reasons, reason => reason.Contains("title nor hash"));
    }

    [Fact]
    public void LowConfidenceBilingualCandidate_RequiresManualConfirmation()
    {
        var report = Advise(new M2ActionInput
        {
            SourceCount = 1,
            TargetLanguagePresent = false,
            CandidateAvailable = true,
            CandidateHealth = "PASS",
            PreferenceSuitability = "RECOMMENDED",
            TitleMatch = true,
            PreferBilingual = true,
            BilingualDetected = true,
            BilingualConfidence = 0.2d
        });

        Assert.Equal(M2ActionNames.Manual, report.Action);
        Assert.Contains(report.Reasons, reason => reason.Contains("low confidence"));
    }

    [Fact]
    public void PresentTargetWithoutHealth_IsManualRatherThanImplicitKeep()
    {
        var report = Advise(new M2ActionInput
        {
            SourceCount = 1,
            TargetLanguagePresent = true
        });

        Assert.Equal(M2ActionNames.Manual, report.Action);
        Assert.Contains(report.Reasons, reason => reason.Contains("health is not known"));
    }

    [Fact]
    public void UnknownState_IsManual()
    {
        var report = Advise(new M2ActionInput
        {
            SourceCount = 1,
            StateKnown = false,
            TargetLanguagePresent = false
        });

        Assert.Equal(M2ActionNames.Manual, report.Action);
    }

    private static M2ActionReport Advise(M2ActionInput input)
    {
        return new M2ActionAdvisor().Advise(input);
    }
}
