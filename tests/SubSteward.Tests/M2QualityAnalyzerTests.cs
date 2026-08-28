using System.Text;
using SubSteward.Plugin.M1;
using SubSteward.Plugin.M2;
using Xunit;

namespace SubSteward.Tests;

public sealed class M2QualityAnalyzerTests
{
    [Fact]
    public void BilingualSrt_IsDetectedWhileHealthRemainsPass()
    {
        var validation = new M1SubtitleValidator().Validate(Encoding.UTF8.GetBytes("1\n00:00:01,000 --> 00:00:02,000\n你好 Hello\n"), "srt");
        var report = new M2QualityAnalyzer().Analyze(validation);

        Assert.Equal("PASS", report.Health);
        Assert.Equal(1, report.TargetLanguageCueCount);
        Assert.Equal(1, report.SecondaryLanguageCueCount);
        Assert.Equal(1, report.BilingualCueCount);
        Assert.True(report.BilingualDetected);
    }

    [Fact]
    public void AssOverrideTags_AreNotCountedAsSecondaryLanguage()
    {
        var validation = new M1SubtitleValidator().Validate(Encoding.UTF8.GetBytes("[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,{\\i1}你好\n"), "ass");
        var report = new M2QualityAnalyzer().Analyze(validation);

        Assert.Equal(1, report.TargetLanguageCueCount);
        Assert.Equal(0, report.SecondaryLanguageCueCount);
        Assert.False(report.BilingualDetected);
        Assert.Equal(1, report.EffectCueCount);
        Assert.Equal("Light", report.EffectStrength);
    }

    [Fact]
    public void IsolatedLatinLetter_IsNotDetectedAsEnglish()
    {
        var validation = new M1SubtitleValidator().Validate(Encoding.UTF8.GetBytes("1\n00:00:01,000 --> 00:00:02,000\n这是 A 计划\n"), "srt");
        var report = new M2QualityAnalyzer().Analyze(validation);

        Assert.True(report.TargetLanguagePresent);
        Assert.False(report.SecondaryLanguagePresent);
        Assert.False(report.BilingualDetected);
    }

    [Theory]
    [InlineData("zh-Hans")]
    [InlineData("zh-Hant")]
    public void ChineseLanguageVariants_AreDetectedAsChineseContent(string language)
    {
        var validation = new M1SubtitleValidator().Validate(
            Encoding.UTF8.GetBytes("1\n00:00:01,000 --> 00:00:02,000\n你好\n"),
            "srt");

        var report = new M2QualityAnalyzer().Analyze(validation, language);

        Assert.Equal("zho", report.TargetLanguage);
        Assert.True(report.TargetLanguagePresent);
        Assert.Equal(1, report.TargetLanguageCueCount);
    }
}
