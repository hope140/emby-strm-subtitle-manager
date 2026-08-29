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
        public string TargetLanguage { get; set; } = "zh-Hans";

        public string SecondaryLanguage { get; set; } = "eng";

        public bool PreferBilingual { get; set; }

        public string FormatOrder { get; set; } = "ass,ssa,srt";

        /// <summary>
        /// Enables the M3 automatic missing-subtitle task. It is deliberately
        /// disabled until an administrator explicitly opts in.
        /// </summary>
        public bool AutomationEnabled { get; set; }

        /// <summary>
        /// Keeps M3 in validation-only mode. No media file is written while
        /// this flag is true, even when AutomationEnabled is true.
        /// </summary>
        public bool AutomationDryRun { get; set; } = true;

        /// <summary>
        /// Explicit media-library allowlist for M3. An empty list fails closed
        /// and prevents the task from querying media items.
        /// </summary>
        public List<string> AutomationLibraryIds { get; set; } = new List<string>();

        /// <summary>
        /// Upper bound for Movie/Episode items examined by one scheduled run.
        /// </summary>
        public int AutomationMaxItemsPerRun { get; set; } = 20;

        /// <summary>
        /// Upper bound for provider Fetch calls attempted for one item. The
        /// runner applies a hard ceiling of three regardless of this value.
        /// </summary>
        public int AutomationMaxCandidateFetchesPerItem { get; set; } = 3;

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
