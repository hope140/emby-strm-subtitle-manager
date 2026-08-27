using System.Collections.Generic;

namespace SubSteward.Plugin.M2
{
    public sealed class M2SubtitleStreamSnapshot
    {
        public bool IsExternal { get; set; }

        public string Language { get; set; }

        public string Title { get; set; }

        public string Path { get; set; }
    }

    public sealed class M2PresenceReport
    {
        public string TargetLanguage { get; set; }

        public string TargetLanguageLabel { get; set; }

        public string RequestedTargetVariant { get; set; }

        public string SecondaryLanguage { get; set; }

        public string SecondaryLanguageLabel { get; set; }

        public bool TargetLanguagePresent { get; set; }

        public bool SecondaryLanguagePresent { get; set; }

        public int InternalTargetLanguageStreamCount { get; set; }

        public int ExternalTargetLanguageStreamCount { get; set; }

        public List<string> DetectedTargetVariants { get; } = new List<string>();
    }
}
