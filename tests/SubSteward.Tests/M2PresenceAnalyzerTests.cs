using System.Collections.Generic;
using SubSteward.Plugin.M2;
using Xunit;

namespace SubSteward.Tests;

public sealed class M2PresenceAnalyzerTests
{
    [Fact]
    public void KnownLanguageCodes_AreRecognized()
    {
        var report = new M2PresenceAnalyzer().Analyze(new[]
        {
            new M2SubtitleStreamSnapshot { IsExternal = false, Language = "chi" },
            new M2SubtitleStreamSnapshot { IsExternal = true, Language = "eng" }
        });

        Assert.True(report.TargetLanguagePresent);
        Assert.True(report.SecondaryLanguagePresent);
        Assert.Equal(1, report.InternalTargetLanguageStreamCount);
        Assert.Equal(0, report.ExternalTargetLanguageStreamCount);
    }

    [Fact]
    public void UnknownExternalSubtitle_UsesChineseTitleFallback()
    {
        var report = new M2PresenceAnalyzer().Analyze(new[]
        {
            new M2SubtitleStreamSnapshot { IsExternal = true, Title = "中文(简英)" }
        });

        Assert.Equal("zho", report.TargetLanguage);
        Assert.Equal("中文", report.TargetLanguageLabel);
        Assert.True(report.TargetLanguagePresent);
        Assert.Equal(1, report.ExternalTargetLanguageStreamCount);
    }

    [Fact]
    public void ExternalSubtitlePath_ContributesExplicitVariantEvidence()
    {
        var report = new M2PresenceAnalyzer().Analyze(new[]
        {
            new M2SubtitleStreamSnapshot
            {
                IsExternal = true,
                Language = "chi",
                Path = "/media/movies/movie.2001.zh-Hans.ass"
            }
        });

        Assert.True(report.TargetLanguagePresent);
        Assert.Equal(1, report.ExternalTargetLanguageStreamCount);
        Assert.Contains("zh-Hans", report.DetectedTargetVariants);
    }

    [Fact]
    public void CommunityAliases_UseCanonicalVariantLabels()
    {
        var report = new M2PresenceAnalyzer().Analyze(
            new[] { new M2SubtitleStreamSnapshot { IsExternal = true, Language = "chi", Title = "简英" } },
            "chs");

        Assert.Equal("zho", report.TargetLanguage);
        Assert.Equal("zh-Hans", report.RequestedTargetVariant);
        Assert.Equal("中文（简体）", report.TargetLanguageLabel);
        Assert.True(report.TargetLanguagePresent);
        Assert.Contains("zh-Hans", report.DetectedTargetVariants);
    }

    [Fact]
    public void ExplicitOppositeVariant_DoesNotSatisfyRequestedVariant()
    {
        var report = new M2PresenceAnalyzer().Analyze(new[]
        {
            new M2SubtitleStreamSnapshot { IsExternal = true, Language = "zh-Hant" }
        }, "zh-Hans");

        Assert.Equal("zh-Hans", report.RequestedTargetVariant);
        Assert.False(report.TargetLanguagePresent);
    }

    [Fact]
    public void UnknownVariantEvidence_RemainsPresentForRequestedVariant()
    {
        var report = new M2PresenceAnalyzer().Analyze(new[]
        {
            new M2SubtitleStreamSnapshot { IsExternal = true, Language = "zho" }
        }, "zh-Hans");

        Assert.True(report.TargetLanguagePresent);
        Assert.Equal("zh-Hans", report.RequestedTargetVariant);
    }

    [Fact]
    public void KnownJapaneseStream_DoesNotUseChineseTitleFallback()
    {
        var report = new M2PresenceAnalyzer().Analyze(new[]
        {
            new M2SubtitleStreamSnapshot { Language = "jpn", Title = "繁體字幕" }
        });

        Assert.False(report.TargetLanguagePresent);
    }
}
