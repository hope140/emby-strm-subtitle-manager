using System.Collections.Generic;

namespace SubSteward.Plugin.M2
{
    public sealed class M2PreferenceOptions
    {
        public bool PreferBilingual { get; set; }

        public string[] FormatOrder { get; set; } = new[] { "ass", "ssa", "srt" };
    }

    public sealed class M2CandidateAssessment
    {
        public string Token { get; set; }

        public bool TitleMatch { get; set; }

        public bool HashMatch { get; set; }

        public M1.M1ValidationResult Validation { get; set; }
    }

    public sealed class M2PreferenceReport
    {
        public string Token { get; set; }

        public int Rank { get; set; } = 1;

        public double Score { get; set; }

        public string Suitability { get; set; }

        public List<string> Reasons { get; } = new List<string>();

        public M2QualityReport Quality { get; set; }
    }
}
