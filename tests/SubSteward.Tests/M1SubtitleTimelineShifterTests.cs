using System;
using System.Linq;
using System.Text;
using SubSteward.Plugin.M1;
using Xunit;

namespace SubSteward.Tests;

public sealed class M1SubtitleTimelineShifterTests
{
    private readonly M1SubtitleTimelineShifter shifter = new();
    private readonly M1SubtitleValidator validator = new();

    [Fact]
    public void Shift_SrtPreservesBomNewlinesAndCueDuration()
    {
        const string text = "1\r\n00:00:01,250 --> 00:00:03,750 position:50%\r\n中文字幕\r\n";
        var body = Encoding.UTF8.GetBytes(text);
        var content = Encoding.UTF8.GetPreamble().Concat(body).ToArray();

        var shifted = shifter.Shift(content, "srt", 1500);

        Assert.True(shifted.Take(3).SequenceEqual(Encoding.UTF8.GetPreamble()));
        var shiftedText = Encoding.UTF8.GetString(shifted, 3, shifted.Length - 3);
        Assert.Contains("00:00:02,750 --> 00:00:05,250 position:50%", shiftedText);
        Assert.Contains("\r\n", shiftedText);
        var validation = validator.Validate(shifted, "srt");
        var cue = Assert.Single(validation.Cues);
        Assert.Equal(2750, cue.StartMilliseconds);
        Assert.Equal(5250, cue.EndMilliseconds);
        Assert.Equal("PASS", validation.Health);
    }

    [Fact]
    public void Shift_AssUsesEventsFormatIndexesAndPreservesText()
    {
        const string text = "[Script Info]\nTitle: sample\n\n[Events]\n"
            + "Format: Layer, End, Start, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
            + "Dialogue: 0, 0:00:03.00, 0:00:01.00,Default,,0,0,0,,中文,with,commas\n";

        var shifted = shifter.Shift(Encoding.UTF8.GetBytes(text), "ass", 1000);
        var shiftedText = Encoding.UTF8.GetString(shifted);

        Assert.Contains("Dialogue: 0, 0:00:04.00, 0:00:02.00,Default,,0,0,0,,中文,with,commas", shiftedText);
        var validation = validator.Validate(shifted, "ass");
        var cue = Assert.Single(validation.Cues);
        Assert.Equal(2000, cue.StartMilliseconds);
        Assert.Equal(4000, cue.EndMilliseconds);
        Assert.Equal("中文,with,commas", cue.Text);
        Assert.Equal("PASS", validation.Health);
    }

    [Fact]
    public void Shift_RejectsOffsetThatWouldMoveCueBeforeZero()
    {
        var content = Encoding.UTF8.GetBytes("1\n00:00:00,400 --> 00:00:01,400\n中文\n");

        var error = Assert.Throws<InvalidOperationException>(() => shifter.Shift(content, "srt", -500));

        Assert.Contains("outside the supported time range", error.Message);
    }

    [Fact]
    public void Shift_AssRequiresCentisecondPrecision()
    {
        var content = Encoding.UTF8.GetBytes(
            "[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"
            + "Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,中文\n");

        var error = Assert.Throws<ArgumentException>(() => shifter.Shift(content, "ass", 15));

        Assert.Contains("10 millisecond increments", error.Message);
    }

    [Fact]
    public void Shift_ZeroOffsetReturnsIdenticalContentClone()
    {
        const string text = "1\n00:00:01,000 --> 00:00:02,000\n中文\n";
        var content = Encoding.UTF8.GetBytes(text);

        var shifted = shifter.Shift(content, "srt", 0);

        Assert.NotSame(content, shifted);
        Assert.True(content.SequenceEqual(shifted));
        var validation = validator.Validate(shifted, "srt");
        Assert.Equal("PASS", validation.Health);
        Assert.Equal(1000, validation.Cues[0].StartMilliseconds);
    }

    [Fact]
    public void Shift_AssWithHoursAboveNinetyNine_ThrowsFriendlyError()
    {
        var content = Encoding.UTF8.GetBytes(
            "[Events]\nFormat: Layer, Start, End, Style, Text\n"
            + "Dialogue: 0,100:00:01.00,100:00:02.00,Default,中文\n");

        var error = Assert.Throws<InvalidOperationException>(() => shifter.Shift(content, "ass", 1000));

        Assert.Contains("hours exceed the supported range", error.Message);
    }
}
