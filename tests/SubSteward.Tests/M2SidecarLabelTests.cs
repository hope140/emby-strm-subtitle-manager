using System.Text;
using SubSteward.Plugin.M1;
using SubSteward.Plugin.M2;
using Xunit;

namespace SubSteward.Tests;

public sealed class M2SidecarLabelTests
{
    [Fact]
    public void RequestedHansVariant_ProducesSimplifiedChineseLabel()
    {
        var label = M2SidecarLabel.Build(CreateValidation("你好"), "zh-Hans");

        Assert.Equal("中文简体", label);
    }

    [Fact]
    public void JapaneseCoverage_ProducesChineseJapaneseLabel()
    {
        var validation = new M1SubtitleValidator().Validate(
            Encoding.UTF8.GetBytes("1\n00:00:01,000 --> 00:00:02,000\n你好こんにちは\n"),
            "srt");

        Assert.Equal("中日双语", M2SidecarLabel.Build(validation, null));
    }

    [Fact]
    public void EnglishContent_WithHansRequest_DoesNotProduceChineseLabel()
    {
        var label = M2SidecarLabel.Build(CreateValidation("Hello world"), "zh-Hans");

        Assert.Null(label);
    }

    [Fact]
    public void RequestedHantVariant_ProducesTraditionalChineseLabel()
    {
        var label = M2SidecarLabel.Build(CreateValidation("繁體中文"), "zh-Hant");

        Assert.Equal("中文繁體", label);
    }

    private static M1ValidationResult CreateValidation(string text)
    {
        return new M1SubtitleValidator().Validate(Encoding.UTF8.GetBytes($"1\n00:00:01,000 --> 00:00:02,000\n{text}\n"), "srt");
    }
}
