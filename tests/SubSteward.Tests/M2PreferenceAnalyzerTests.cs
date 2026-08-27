using System.Text;
using SubSteward.Plugin.M1;
using SubSteward.Plugin.M2;
using Xunit;

namespace SubSteward.Tests;

public sealed class M2PreferenceAnalyzerTests
{
    [Fact]
    public void HashMatch_IsRankedAboveTitleMatch()
    {
        var ranked = new M2PreferenceAnalyzer().Rank(new[]
        {
            new M2CandidateAssessment { Token = "title", TitleMatch = true, Validation = CreateChineseSrt() },
            new M2CandidateAssessment { Token = "hash", TitleMatch = true, HashMatch = true, Validation = CreateChineseSrt() }
        });

        Assert.Equal(2, ranked.Count);
        Assert.Equal("hash", ranked[0].Token);
        Assert.Equal("RECOMMENDED", ranked[0].Suitability);
        Assert.Equal(1, ranked[0].Rank);
        Assert.True(ranked[0].Score > ranked[1].Score);
    }

    [Fact]
    public void CandidateWithoutTitleOrHashBinding_IsNotRecommended()
    {
        var report = new M2PreferenceAnalyzer().Evaluate(CreateChineseSrt(), token: null);

        Assert.Equal(1, report.Rank);
        Assert.Equal("NOT_RECOMMENDED", report.Suitability);
        Assert.Contains(report.Reasons, reason => reason.Contains("title or hash", System.StringComparison.OrdinalIgnoreCase));
    }

    [Fact]
    public void FailedHealth_IsNotUsable()
    {
        var report = new M2PreferenceAnalyzer().Evaluate(CreateInvalidTimeline(), token: "bad");

        Assert.False(report.Quality.IsUsable);
        Assert.Equal("NOT_RECOMMENDED", report.Suitability);
    }

    private static M1ValidationResult CreateChineseSrt()
    {
        return new M1SubtitleValidator().Validate(
            Encoding.UTF8.GetBytes("1\n00:00:01,000 --> 00:00:02,000\n你好\n"),
            "srt");
    }

    private static M1ValidationResult CreateInvalidTimeline()
    {
        return new M1SubtitleValidator().Validate(
            Encoding.UTF8.GetBytes("1\n00:00:02,000 --> 00:00:01,000\n无效\n"),
            "srt");
    }
}
