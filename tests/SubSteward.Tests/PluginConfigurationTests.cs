using System.IO;
using System.Xml.Serialization;
using SubSteward.Plugin;
using Xunit;

namespace SubSteward.Tests;

public sealed class PluginConfigurationTests
{
    [Fact]
    public void LibraryOverride_RoundTripsThroughXmlConfiguration()
    {
        var configuration = new PluginConfiguration();
        configuration.LibraryOverrides.Add(new LibraryPreferenceOverride
        {
            LibraryId = "0123456789abcdef0123456789abcdef",
            LibraryName = "Animation",
            Enabled = true,
            TargetLanguage = "zh-CN",
            SecondaryLanguage = "jpn",
            PreferBilingual = true,
            FormatOrder = "ass,ssa,srt"
        });

        var serializer = new XmlSerializer(typeof(PluginConfiguration));
        using var stream = new MemoryStream();
        serializer.Serialize(stream, configuration);
        stream.Position = 0;

        var restored = Assert.IsType<PluginConfiguration>(serializer.Deserialize(stream));
        var libraryOverride = Assert.Single(restored.LibraryOverrides);

        Assert.True(libraryOverride.Enabled);
        Assert.Equal("0123456789abcdef0123456789abcdef", libraryOverride.LibraryId);
        Assert.Equal("Animation", libraryOverride.LibraryName);
        Assert.Equal("zh-CN", libraryOverride.TargetLanguage);
        Assert.Equal("jpn", libraryOverride.SecondaryLanguage);
        Assert.True(libraryOverride.PreferBilingual);
        Assert.Equal("ass,ssa,srt", libraryOverride.FormatOrder);
    }
}
