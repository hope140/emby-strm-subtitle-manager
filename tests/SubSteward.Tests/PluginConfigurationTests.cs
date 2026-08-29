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

    [Fact]
    public void AutomationConfiguration_RoundTripsThroughXmlConfiguration()
    {
        var configuration = new PluginConfiguration
        {
            AutomationEnabled = true,
            AutomationDryRun = false,
            AutomationMaxItemsPerRun = 12,
            AutomationMaxCandidateFetchesPerItem = 2
        };
        configuration.AutomationLibraryIds.Add("0123456789abcdef0123456789abcdef");

        var serializer = new XmlSerializer(typeof(PluginConfiguration));
        using var stream = new MemoryStream();
        serializer.Serialize(stream, configuration);
        stream.Position = 0;

        var restored = Assert.IsType<PluginConfiguration>(serializer.Deserialize(stream));

        Assert.True(restored.AutomationEnabled);
        Assert.False(restored.AutomationDryRun);
        Assert.Equal(12, restored.AutomationMaxItemsPerRun);
        Assert.Equal(2, restored.AutomationMaxCandidateFetchesPerItem);
        Assert.Equal("0123456789abcdef0123456789abcdef", Assert.Single(restored.AutomationLibraryIds));
    }

    [Fact]
    public void AutomationConfiguration_DefaultsClosedAndDryRun()
    {
        var configuration = new PluginConfiguration();

        Assert.False(configuration.AutomationEnabled);
        Assert.True(configuration.AutomationDryRun);
        Assert.Empty(configuration.AutomationLibraryIds);
        Assert.Equal(20, configuration.AutomationMaxItemsPerRun);
        Assert.Equal(3, configuration.AutomationMaxCandidateFetchesPerItem);
    }
}
