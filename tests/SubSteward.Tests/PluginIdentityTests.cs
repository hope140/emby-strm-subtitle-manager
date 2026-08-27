using System.Reflection;
using SubSteward.Plugin;
using SubStewardPlugin = SubSteward.Plugin.Plugin;
using Xunit;

namespace SubSteward.Tests;

public sealed class PluginIdentityTests
{
    [Fact]
    public void PluginIdentity_UsesTheApprovedBrandAndStableId()
    {
        Assert.Equal("SubSteward", PluginIdentity.DisplayName);
        Assert.Equal("Subtitle Automation for Emby", PluginIdentity.Description);
        Assert.Equal("20b47482-cb89-42d2-a6e0-5b87fd9b7858", PluginIdentity.Id.ToString());
    }

    [Fact]
    public void AssemblyMetadata_UsesTheApprovedBrandAndDescription()
    {
        var assembly = typeof(SubStewardPlugin).Assembly;

        Assert.Equal("SubSteward", assembly.GetCustomAttribute<AssemblyTitleAttribute>()?.Title);
        Assert.Equal("SubSteward", assembly.GetCustomAttribute<AssemblyProductAttribute>()?.Product);
        Assert.Equal("Subtitle Automation for Emby", assembly.GetCustomAttribute<AssemblyDescriptionAttribute>()?.Description);
    }

}
