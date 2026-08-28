using System.Collections.Generic;
using MediaBrowser.Model.Plugins;

namespace SubSteward.Plugin
{
    /// <summary>
    /// Holds user configuration for the plugin. V1 options are intentionally added
    /// only after M0 confirms the public Plugin API on the target server.
    /// </summary>
    public sealed class PluginConfiguration : BasePluginConfiguration
    {
        public string TargetLanguage { get; set; } = "zho";

        public string SecondaryLanguage { get; set; } = "eng";

        public bool PreferBilingual { get; set; }

        public string FormatOrder { get; set; } = "ass,ssa,srt";

        public List<LibraryPreferenceOverride> LibraryOverrides { get; set; } = new List<LibraryPreferenceOverride>();
    }

    /// <summary>
    /// Overrides the global subtitle preference set for one Emby media library.
    /// Disabled entries are retained so an administrator can temporarily return
    /// a library to inheritance without losing the previous values.
    /// </summary>
    public sealed class LibraryPreferenceOverride
    {
        public string LibraryId { get; set; }

        public string LibraryName { get; set; }

        public bool Enabled { get; set; }

        public string TargetLanguage { get; set; }

        public string SecondaryLanguage { get; set; }

        public bool PreferBilingual { get; set; }

        public string FormatOrder { get; set; }
    }
}
