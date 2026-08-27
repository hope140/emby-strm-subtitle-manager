using System;
using System.Collections.Generic;
using System.Linq;

namespace SubSteward.Plugin.M2
{
    public sealed class M2PreferenceAnalyzer
    {
        public List<M2PreferenceReport> Rank(
            IEnumerable<M2CandidateAssessment> candidates,
            string targetLanguage = null,
            string secondaryLanguage = null,
            M2PreferenceOptions options = null)
        {
            if (candidates == null)
            {
                throw new ArgumentNullException(nameof(candidates));
            }

            var effectiveOptions = options ?? new M2PreferenceOptions();
            var reports = new List<M2PreferenceReport>();
            foreach (var candidate in candidates)
            {
                if (candidate == null || candidate.Validation == null)
                {
                    continue;
                }

                reports.Add(EvaluateCore(candidate.Validation, targetLanguage, secondaryLanguage, candidate.Token, candidate.TitleMatch, candidate.HashMatch, effectiveOptions));
            }

            return reports
                .OrderByDescending(report => report.Suitability == "NOT_RECOMMENDED" ? 0 : 1)
                .ThenByDescending(report => report.Quality.Health == "PASS" ? 1 : 0)
                .ThenByDescending(report => report.Quality.TargetLanguageConfidence)
                .ThenByDescending(report => report.Score)
                .ThenBy(report => report.Token ?? string.Empty, StringComparer.Ordinal)
                .Select((report, index) =>
                {
                    report.Rank = index + 1;
                    return report;
                })
                .ToList();
        }

        public M2PreferenceReport Evaluate(
            M1.M1ValidationResult validation,
            string targetLanguage = null,
            string secondaryLanguage = null,
            string token = null,
            bool titleMatch = false,
            bool hashMatch = false,
            M2PreferenceOptions options = null)
        {
            if (validation == null)
            {
                throw new ArgumentNullException(nameof(validation));
            }

            return EvaluateCore(validation, targetLanguage, secondaryLanguage, token, titleMatch, hashMatch, options ?? new M2PreferenceOptions());
        }

        private static M2PreferenceReport EvaluateCore(
            M1.M1ValidationResult validation,
            string targetLanguage,
            string secondaryLanguage,
            string token,
            bool titleMatch,
            bool hashMatch,
            M2PreferenceOptions options)
        {
            var quality = new M2QualityAnalyzer().Analyze(validation, targetLanguage, secondaryLanguage);
            var report = new M2PreferenceReport
            {
                Token = token,
                Quality = quality
            };

            if (!quality.IsUsable)
            {
                report.Score = 0d;
                report.Suitability = "NOT_RECOMMENDED";
                report.Reasons.Add("Health FAILED and the candidate was eliminated");
                return report;
            }

            double score = 0d;
            if (string.Equals(quality.Health, "PASS", StringComparison.Ordinal))
            {
                score += 20d;
                report.Reasons.Add("Health is PASS");
            }
            else
            {
                score += 8d;
                report.Reasons.Add("Health is WARNING; inspect it before installation");
            }

            if (hashMatch)
            {
                score += 35d;
                report.Reasons.Add("The provider reported a hash match");
            }
            else if (titleMatch)
            {
                score += 22d;
                report.Reasons.Add("Only metadata title matching is available");
            }
            else
            {
                score -= 10d;
                report.Reasons.Add("No provider hash or title binding is available");
            }

            score += quality.TargetLanguageConfidence * 25d;
            if (quality.TargetLanguagePresent)
            {
                report.Reasons.Add("Target-language text was detected in subtitle content");
            }

            if (options.PreferBilingual && quality.BilingualDetected)
            {
                score += 10d + Math.Min(5d, quality.BilingualConfidence * 10d);
                report.Reasons.Add("Bilingual content matches the configured preference");
            }

            var formatScore = FormatScore(quality.Format, options.FormatOrder);
            score += formatScore;
            if (formatScore > 0d)
            {
                report.Reasons.Add("Format " + quality.Format.ToUpperInvariant() + " matches a preferred format order");
            }

            report.Score = Math.Min(100d, Math.Round(score, 1));
            if (report.Score >= 80d && quality.TargetLanguagePresent)
            {
                report.Suitability = "RECOMMENDED";
            }
            else if (report.Score >= 55d && quality.TargetLanguagePresent)
            {
                report.Suitability = "ACCEPTABLE";
            }
            else
            {
                report.Suitability = "NOT_RECOMMENDED";
                if (!quality.TargetLanguagePresent)
                {
                    report.Reasons.Add("Target-language text was not detected in subtitle content");
                }
                else if (!titleMatch && !hashMatch)
                {
                    report.Reasons.Add("A candidate must have title or hash evidence before installation");
                }
                else if (report.Score < 55d)
                {
                    report.Reasons.Add("Weak evidence or missing preference match");
                }
            }

            return report;
        }

        private static double FormatScore(string format, string[] formatOrder)
        {
            if (formatOrder == null || formatOrder.Length == 0)
            {
                return 0d;
            }

            for (var index = 0; index < formatOrder.Length; index++)
            {
                if (string.Equals(format, formatOrder[index], StringComparison.OrdinalIgnoreCase))
                {
                    var remaining = formatOrder.Length - index;
                    return Math.Max(3d, remaining * 4d);
                }
            }

            return 0d;
        }
    }
}
