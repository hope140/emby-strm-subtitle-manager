using SubSteward.Plugin.M2;
using Xunit;

namespace SubSteward.Tests;

public sealed class M2OptionsTests
{
    [Fact]
    public void FormatOrder_IsNormalizedAndDeduplicated()
    {
        var options = M2Options.ParsePreferenceOptions(true, ".srt, ASS; ssa,srt");

        Assert.True(options.PreferBilingual);
        Assert.Equal(new[] { "srt", "ass", "ssa" }, options.FormatOrder);
    }

    [Fact]
    public void BlankLanguages_UseSafeDefaults()
    {
        Assert.Equal("zho", M2Options.NormalizeTargetLanguage("  "));
        Assert.Null(M2Options.ParseTargetLanguage("zho").Variant);
        Assert.Equal("中文", M2Options.ParseTargetLanguage("zho").Label);
        Assert.Equal("eng", M2Options.NormalizeSecondaryLanguage(null));
    }

    [Fact]
    public void BlankFormatOrder_UsesDefault()
    {
        Assert.Equal(new[] { "ass", "ssa", "srt" }, M2Options.ParseFormatOrder(""));
    }

    [Fact]
    public void CommunityChineseAliases_MapToCanonicalVariants()
    {
        var simplified = M2Options.ParseTargetLanguage("chs");
        Assert.Equal("zho", simplified.Code);
        Assert.Equal("zh-Hans", simplified.Variant);
        Assert.Equal("中文（简体）", simplified.Label);

        var traditional = M2Options.ParseTargetLanguage("cht");
        Assert.Equal("zho", traditional.Code);
        Assert.Equal("zh-Hant", traditional.Variant);
        Assert.Equal("中文（繁体）", traditional.Label);
    }

    [Fact]
    public void SidecarTags_UseEmbyFriendlyRegionalLanguageCodes()
    {
        Assert.Equal("zh-CN", M2Options.ResolveSubtitleLanguageTag("zh-Hans", "zho"));
        Assert.Equal("zh-TW", M2Options.ResolveSubtitleLanguageTag("zh-Hant", "zho"));
        Assert.Equal("zho", M2Options.ResolveSubtitleLanguageTag(null, "zho"));
        Assert.Equal("zh-CN", M2Options.ResolveSubtitleLanguageTag(null, "zh-Hans"));
    }

    [Fact]
    public void CanonicalLanguageValues_PreserveChineseVariants()
    {
        Assert.Equal("zh-Hans", M2Options.CanonicalizeTargetLanguage("chs"));
        Assert.Equal("zh-Hant", M2Options.CanonicalizeTargetLanguage("zh-TW"));
        Assert.Equal("zh-Hant", M2Options.CanonicalizeSecondaryLanguage("cht"));
    }
}
