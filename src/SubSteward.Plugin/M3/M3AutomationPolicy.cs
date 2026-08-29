using System;

namespace SubSteward.Plugin.M3
{
    /// <summary>
    /// Fail-closed gates for M3 automatic missing-subtitle supplementation.
    /// This class has no Emby dependency so the safety rules can be tested
    /// independently from provider, filesystem and scheduled-task behavior.
    /// </summary>
    public sealed class M3AutomationPolicy
    {
        public const int MinimumAutomaticMatchScore = 85;

        public M3EligibilityReport EvaluateItem(M3EligibilityInput input)
        {
            if (input == null)
            {
                throw new ArgumentNullException(nameof(input));
            }

            if (!input.AutomationEnabled)
            {
                return Report(false, M3AutomationDecisionNames.Skip, "M3 automation is disabled");
            }

            if (!input.LibraryAuthorized)
            {
                return Report(false, M3AutomationDecisionNames.Skip, "The media library is not in the M3 allowlist");
            }

            if (input.IsIgnored)
            {
                return Report(false, M3AutomationDecisionNames.Skip, "The item is explicitly ignored");
            }

            if (!input.StateKnown)
            {
                return Report(false, M3AutomationDecisionNames.Manual, "Required subtitle state is not known");
            }

            if (input.SourceCount != 1)
            {
                return Report(false, M3AutomationDecisionNames.Manual, "M3 requires exactly one MediaSource");
            }

            if (!input.HasSourceIdentity)
            {
                return Report(false, M3AutomationDecisionNames.Manual, "The selected MediaSource has no stable identity");
            }

            if (!input.TargetLanguagePresent.HasValue)
            {
                return Report(false, M3AutomationDecisionNames.Manual, "Target-language presence is not known");
            }

            if (input.TargetLanguagePresent.Value)
            {
                return Report(false, M3AutomationDecisionNames.Skip, "Target-language subtitle is already present");
            }

            if (!input.HasSafeWriteAnchor)
            {
                return Report(false, M3AutomationDecisionNames.Manual, "The selected source has no safe local subtitle write anchor");
            }

            return Report(true, M3AutomationDecisionNames.Eligible, "Item is eligible for M3 automatic supplementation");
        }

        public M3CandidateGateReport EvaluateCandidateMetadata(M3CandidateMetadataInput input)
        {
            if (input == null)
            {
                throw new ArgumentNullException(nameof(input));
            }

            var report = new M3CandidateGateReport
            {
                MatchScore = CalculateMatchScore(input)
            };

            var structuredMatch = input.ReleaseYearMatch || input.EpisodeMatch;
            if (input.IsStrm)
            {
                // A STRM file is a playback pointer, not the video bytes. Its
                // own file hash is not useful, so STRM automation uses the
                // media title plus year/episode metadata instead.
                if (input.LikelyNonFullRelease)
                {
                    return Reject(report, "STRM candidate appears to be a short-form source");
                }

                if (!input.TitleMatch || !structuredMatch || report.MatchScore < MinimumAutomaticMatchScore)
                {
                    return Reject(report, "STRM automatic matching requires title and year/episode metadata");
                }
            }
            else if (report.MatchScore < MinimumAutomaticMatchScore)
            {
                return Reject(report, "Automatic matching requires a provider hash or title plus year/episode metadata");
            }

            if (input.LikelyNonFullRelease && !input.HashMatch)
            {
                return Reject(report, "Candidate appears to be a short-form source without a hash match");
            }

            if (input.LanguageMismatch)
            {
                return Reject(report, "Candidate language does not match the configured target language");
            }

            if (input.VariantMismatch)
            {
                return Reject(report, "Candidate language variant does not match the configured target variant");
            }

            report.IsEligible = true;
            report.Decision = M3AutomationDecisionNames.Eligible;
            report.Reasons.Add(input.IsStrm
                ? "Candidate has title and year/episode evidence suitable for STRM matching"
                : input.HashMatch
                    ? "Candidate has a provider media-fingerprint match and matching language metadata"
                    : "Candidate has title and year/episode evidence and matching language metadata");
            return report;
        }

        public M3CandidateGateReport EvaluateCandidate(M3CandidateGateInput input)
        {
            if (input == null)
            {
                throw new ArgumentNullException(nameof(input));
            }

            var report = EvaluateCandidateMetadata(input);
            if (!report.IsEligible)
            {
                return report;
            }

            if (!string.Equals(input.Health, "PASS", StringComparison.OrdinalIgnoreCase))
            {
                if (string.Equals(input.Health, "WARNING", StringComparison.OrdinalIgnoreCase))
                {
                    return Manual(report, "Candidate Health is WARNING; human review is required");
                }

                return Reject(report, "Candidate Health is not PASS");
            }

            if (!input.TargetLanguagePresent)
            {
                return Reject(report, "Fetched subtitle content does not contain the configured target language");
            }

            if (string.Equals(input.PreferenceSuitability, "NOT_RECOMMENDED", StringComparison.OrdinalIgnoreCase))
            {
                return Reject(report, "Preference analysis does not recommend this candidate");
            }

            if (!string.Equals(input.PreferenceSuitability, "RECOMMENDED", StringComparison.OrdinalIgnoreCase))
            {
                return Manual(report, "Preference analysis is not strong enough for automatic installation");
            }

            report.IsEligible = true;
            report.Decision = M3AutomationDecisionNames.Eligible;
            report.Reasons.Add("Candidate passed the M3 Health, language and preference gates");
            return report;
        }

        public static int CalculateMatchScore(M3CandidateMetadataInput input)
        {
            if (input == null)
            {
                throw new ArgumentNullException(nameof(input));
            }

            if (!input.IsStrm && input.HashMatch)
            {
                return 100;
            }

            if (input.TitleMatch && (input.ReleaseYearMatch || input.EpisodeMatch))
            {
                return 85;
            }

            return input.TitleMatch ? 70 : 0;
        }

        private static M3EligibilityReport Report(bool eligible, string decision, string reason)
        {
            var report = new M3EligibilityReport
            {
                IsEligible = eligible,
                Decision = decision
            };
            report.Reasons.Add(reason);
            return report;
        }

        private static M3CandidateGateReport Reject(M3CandidateGateReport report, string reason)
        {
            report.IsEligible = false;
            report.Decision = M3AutomationDecisionNames.Reject;
            report.Reasons.Add(reason);
            return report;
        }

        private static M3CandidateGateReport Manual(M3CandidateGateReport report, string reason)
        {
            report.IsEligible = false;
            report.Decision = M3AutomationDecisionNames.Manual;
            report.Reasons.Add(reason);
            return report;
        }
    }
}
