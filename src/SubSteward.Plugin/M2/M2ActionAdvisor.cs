using System;

namespace SubSteward.Plugin.M2
{
    /// <summary>
    /// Applies the conservative M2 action policy. It only returns advice;
    /// Repair, Upgrade and subtitle installation remain outside M2 automation.
    /// </summary>
    public sealed class M2ActionAdvisor
    {
        private const double LowBilingualConfidenceThreshold = 0.5d;

        public M2ActionReport Advise(M2ActionInput input)
        {
            if (input == null)
            {
                throw new ArgumentNullException(nameof(input));
            }

            if (input.IsIgnored)
            {
                return Report(M2ActionNames.Ignore, "The item was explicitly marked to be ignored");
            }

            if (!input.StateKnown)
            {
                return Report(M2ActionNames.Manual, "Required subtitle state is not known");
            }

            if (input.SourceCount != 1)
            {
                return Report(M2ActionNames.Manual, "M2 action requires exactly one MediaSource");
            }

            if (input.TargetLanguagePresent == true)
            {
                return AdviseForExistingTarget(input);
            }

            if (input.TargetLanguagePresent == null && !input.CandidateAvailable)
            {
                return Report(M2ActionNames.Manual, "Target-language presence is not known");
            }

            if (!input.CandidateAvailable)
            {
                return Report(M2ActionNames.Search, "Target-language subtitle is missing and no usable candidate is available");
            }

            return AdviseForCandidate(input);
        }

        private static M2ActionReport AdviseForExistingTarget(M2ActionInput input)
        {
            if (string.Equals(input.ExistingTargetHealth, "PASS", StringComparison.OrdinalIgnoreCase))
            {
                return Report(M2ActionNames.Keep, "Target-language subtitle is present and its health is PASS");
            }

            if (string.Equals(input.ExistingTargetHealth, "WARNING", StringComparison.OrdinalIgnoreCase))
            {
                return Report(M2ActionNames.Manual, "Target-language subtitle is present but its health is WARNING; inspect it manually");
            }

            if (string.Equals(input.ExistingTargetHealth, "FAIL", StringComparison.OrdinalIgnoreCase))
            {
                return Report(M2ActionNames.Manual, "Target-language subtitle is present but failed health checks; automatic Repair is disabled");
            }

            return Report(M2ActionNames.Manual, "Target-language subtitle is present but its health is not known");
        }

        private static M2ActionReport AdviseForCandidate(M2ActionInput input)
        {
            if (string.IsNullOrWhiteSpace(input.CandidateHealth))
            {
                return Report(M2ActionNames.Manual, "Candidate health is not known");
            }

            if (string.Equals(input.CandidateHealth, "FAIL", StringComparison.OrdinalIgnoreCase))
            {
                return Report(M2ActionNames.Search, "Candidate Health is FAIL; search for another candidate");
            }

            if (!string.Equals(input.CandidateHealth, "PASS", StringComparison.OrdinalIgnoreCase)
                && !string.Equals(input.CandidateHealth, "WARNING", StringComparison.OrdinalIgnoreCase))
            {
                return Report(M2ActionNames.Manual, "Candidate health has an unknown value");
            }

            if (string.Equals(input.CandidateHealth, "WARNING", StringComparison.OrdinalIgnoreCase))
            {
                return Report(M2ActionNames.Manual, "Candidate Health is WARNING; inspect it manually before installation");
            }

            if (!input.TitleMatch && !input.HashMatch)
            {
                return Report(M2ActionNames.Search, "Candidate has neither title nor hash binding to the selected Item");
            }

            if (string.IsNullOrWhiteSpace(input.PreferenceSuitability))
            {
                return Report(M2ActionNames.Manual, "Candidate preference suitability is not known");
            }

            if (string.Equals(input.PreferenceSuitability, "NOT_RECOMMENDED", StringComparison.OrdinalIgnoreCase))
            {
                return Report(M2ActionNames.Search, "Candidate is not recommended by Preference analysis; search for another candidate");
            }

            if (!string.Equals(input.PreferenceSuitability, "RECOMMENDED", StringComparison.OrdinalIgnoreCase)
                && !string.Equals(input.PreferenceSuitability, "ACCEPTABLE", StringComparison.OrdinalIgnoreCase))
            {
                return Report(M2ActionNames.Manual, "Candidate preference suitability has an unknown value");
            }

            if (input.PreferBilingual && input.BilingualDetected && input.BilingualConfidence < LowBilingualConfidenceThreshold)
            {
                return Report(M2ActionNames.Manual, "Bilingual detection has low confidence; human confirmation is required");
            }

            return Report(M2ActionNames.Manual, "Candidate passed the M2 checks; human confirmation is required before installation");
        }

        private static M2ActionReport Report(string action, string reason)
        {
            var report = new M2ActionReport { Action = action };
            report.Reasons.Add(reason);
            return report;
        }
    }
}
