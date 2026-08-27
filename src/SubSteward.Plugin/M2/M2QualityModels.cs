namespace SubSteward.Plugin.M2
{
    public sealed class M2QualityReport
    {
        public string TargetLanguage { get; set; }

        public string SecondaryLanguage { get; set; }

        public string Format { get; set; }

        public string Encoding { get; set; }

        public string Health { get; set; }

        public bool IsUsable
        {
            get { return string.Equals(Health, "PASS", System.StringComparison.Ordinal) || string.Equals(Health, "WARNING", System.StringComparison.Ordinal); }
        }

        public int CueCount { get; set; }

        public int TargetLanguageCueCount { get; set; }

        public int SecondaryLanguageCueCount { get; set; }

        public int JapaneseCueCount { get; set; }

        public int BilingualCueCount { get; set; }

        public bool TargetLanguagePresent { get; set; }

        public double TargetLanguageConfidence { get; set; }

        public bool SecondaryLanguagePresent { get; set; }

        public bool BilingualDetected { get; set; }

        public double BilingualConfidence { get; set; }

        public string EffectStrength { get; set; }

        public int EffectCueCount { get; set; }

        public int StyledCueCount { get; set; }
    }
}
