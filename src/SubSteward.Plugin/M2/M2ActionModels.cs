using System.Collections.Generic;

namespace SubSteward.Plugin.M2
{
    public static class M2ActionNames
    {
        public const string Keep = "KEEP";

        public const string Repair = "REPAIR";

        public const string Search = "SEARCH";

        public const string Upgrade = "UPGRADE";

        public const string Manual = "MANUAL";

        public const string Ignore = "IGNORE";
    }

    /// <summary>
    /// Pure input for the M2 action decision. A null value means that the
    /// corresponding state was not measured, rather than that it is false.
    /// </summary>
    public sealed class M2ActionInput
    {
        public int SourceCount { get; set; }

        public bool? TargetLanguagePresent { get; set; }

        public string ExistingTargetHealth { get; set; }

        public bool CandidateAvailable { get; set; }

        public string CandidateHealth { get; set; }

        public string PreferenceSuitability { get; set; }

        public bool TitleMatch { get; set; }

        public bool HashMatch { get; set; }

        public bool PreferBilingual { get; set; }

        public bool BilingualDetected { get; set; }

        public double BilingualConfidence { get; set; }

        public bool IsIgnored { get; set; }

        public bool StateKnown { get; set; } = true;
    }

    public sealed class M2ActionReport
    {
        public string Action { get; set; }

        public List<string> Reasons { get; } = new List<string>();
    }
}
