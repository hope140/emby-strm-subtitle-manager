using System.Text;
using SubSteward.Plugin.M1;
using Xunit;

namespace SubSteward.Tests;

public sealed class M1SubtitleValidatorTests
{
    [Fact]
    public void ValidSrt_IsPassAndProducesCues()
    {
        var result = new M1SubtitleValidator().Validate(Encoding.UTF8.GetBytes("1\n00:00:01,000 --> 00:00:02,500\n你好\n"), "srt");

        Assert.Equal("PASS", result.Health);
        Assert.Equal("UTF-8", result.Encoding);
        Assert.Single(result.Cues);
        Assert.Equal(1000, result.Cues[0].StartMilliseconds);
        Assert.Equal(2500, result.Cues[0].EndMilliseconds);
    }

    [Fact]
    public void ValidAss_IsPassAndPreservesDialogueTextForPreview()
    {
        const string ass = "[Script Info]\n[V4+ Styles]\n[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:01.00,0:00:02.50,Default,{\\i1}你好\n";

        var result = new M1SubtitleValidator().Validate(Encoding.UTF8.GetBytes(ass), "ass");

        Assert.Equal("PASS", result.Health);
        Assert.Single(result.Cues);
        Assert.Equal("{\\i1}你好", result.Cues[0].Text);
    }

    [Fact]
    public void InvalidTimeline_IsFailAndDoesNotProduceAnArtifactCandidate()
    {
        var result = new M1SubtitleValidator().Validate(Encoding.UTF8.GetBytes("1\n00:00:02,000 --> 00:00:01,000\n坏时间轴\n"), "srt");

        Assert.Equal("FAIL", result.Health);
        Assert.Contains("timeline", string.Join(" ", result.Reasons), System.StringComparison.OrdinalIgnoreCase);
        Assert.False(result.IsUsable);
    }

    [Fact]
    public void Utf16Bom_IsDecodedForPreview()
    {
        var utf16 = new UnicodeEncoding(false, true).GetBytes("1\n00:00:01,000 --> 00:00:02,000\nUTF16\n");
        var content = new byte[utf16.Length + 2];
        content[0] = 0xFF;
        content[1] = 0xFE;
        System.Buffer.BlockCopy(utf16, 0, content, 2, utf16.Length);

        var result = new M1SubtitleValidator().Validate(content, ".srt");

        Assert.Equal("PASS", result.Health);
        Assert.Equal("UTF-16 LE", result.Encoding);
    }

    [Fact]
    public void EnglishSrt_IsNotDetectedAsChineseContent()
    {
        var result = new M1SubtitleValidator().Validate(Encoding.UTF8.GetBytes("1\n00:00:01,000 --> 00:00:02,000\nGood luck, Chihiro\n"), "srt");

        Assert.Equal("PASS", result.Health);
        Assert.False(result.HasHanCharacters);
    }

    [Fact]
    public void InconsistentSrtNumbering_IsWarning()
    {
        var result = new M1SubtitleValidator().Validate(Encoding.UTF8.GetBytes("3\n00:00:01,000 --> 00:00:02,000\n你好\n"), "srt");

        Assert.Equal("WARNING", result.Health);
        Assert.True(result.HasSrtNumberingIssue);
    }

    [Fact]
    public void NulCharacter_IsWarning()
    {
        var result = new M1SubtitleValidator().Validate(Encoding.UTF8.GetBytes("1\n00:00:01,000 --> 00:00:02,000\n坏字幕\0\n"), "srt");

        Assert.Equal("WARNING", result.Health);
        Assert.True(result.HasNulCharacter);
    }

    [Fact]
    public void RepeatedAssOverrideIssues_AreReportedOnlyOnce()
    {
        const string ass = "[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,{\\i1你好\nDialogue: 0,0:00:03.00,0:00:04.00,Default,{\\i1再来\n";
        var result = new M1SubtitleValidator().Validate(Encoding.UTF8.GetBytes(ass), "ass");

        Assert.Equal("WARNING", result.Health);
        Assert.True(result.HasAssOverrideTagIssue);
        Assert.Equal(1, System.Linq.Enumerable.Count(result.Reasons, reason => reason == "ASS dialogue has an unbalanced override tag"));
    }
}
