using System;

namespace SubSteward.Plugin
{
    /// <summary>
    /// Stable product identity shared by the Emby entry point and basic tests.
    /// </summary>
    public static class PluginIdentity
    {
        public static readonly Guid Id = new Guid("20b47482-cb89-42d2-a6e0-5b87fd9b7858");

        public const string DisplayName = "SubSteward";

        public const string Description = "Subtitle Automation for Emby";
    }
}
