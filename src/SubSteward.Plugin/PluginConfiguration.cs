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
    }
}
