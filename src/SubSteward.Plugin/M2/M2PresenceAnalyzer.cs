using System;
using System.Collections.Generic;
using System.Text.RegularExpressions;

namespace SubSteward.Plugin.M2
{
    public sealed class M2PresenceAnalyzer
    {
        private static readonly Regex TagSeparator = new Regex("[\\.\\-_\\[\\] ()]+", RegexOptions.CultureInvariant);

        public M2PresenceReport Analyze(
            IEnumerable<M2SubtitleStreamSnapshot> streams,
            string targetLanguage = "zho",
            string secondaryLanguage = "eng")
        {
            var target = M2Language.Parse(targetLanguage, M2Language.Chinese);
            var secondary = M2Language.Parse(secondaryLanguage, M2Language.English);
            var report = new M2PresenceReport
            {
                TargetLanguage = target.Code,
                TargetLanguageLabel = target.Label,
                RequestedTargetVariant = target.Variant,
                SecondaryLanguage = secondary.Code,
                SecondaryLanguageLabel = secondary.Label
            };

            if (streams != null)
            {
                foreach (var stream in streams)
                {
                    if (stream == null)
                    {
                        continue;
                    }

                    if (MatchesConfiguredLanguage(stream, target))
                    {
                        report.TargetLanguagePresent = true;
                        var variant = DetectStreamVariant(stream);
                        if (!string.IsNullOrWhiteSpace(variant) && !report.DetectedTargetVariants.Contains(variant))
                        {
                            report.DetectedTargetVariants.Add(variant);
                        }

                        if (stream.IsExternal)
                        {
                            report.ExternalTargetLanguageStreamCount++;
                        }
                        else
                        {
                            report.InternalTargetLanguageStreamCount++;
                        }
                    }

                    if (MatchesConfiguredLanguage(stream, secondary))
                    {
                        report.SecondaryLanguagePresent = true;
                    }
                }
            }

            return report;
        }

        public bool MatchesConfiguredLanguage(M2SubtitleStreamSnapshot stream, string expectedLanguage)
        {
            if (stream == null)
            {
                return false;
            }

            return MatchesConfiguredLanguage(stream, M2Language.Parse(expectedLanguage, M2Language.Chinese));
        }

        private static bool MatchesConfiguredLanguage(M2SubtitleStreamSnapshot stream, M2LanguageSelection expected)
        {
            var evidence = BuildEvidence(stream);
            return MatchesLanguage(expected.Code, stream.Language, evidence)
                && M2Language.StreamMatchesVariant(stream, expected.Variant);
        }

        private static bool MatchesLanguage(string expectedLanguage, string actualLanguage, string title)
        {
            if (!string.IsNullOrWhiteSpace(actualLanguage))
            {
                return IsKnownMatch(expectedLanguage, actualLanguage);
            }

            return IsTextualFallbackMatch(expectedLanguage, title);
        }

        private static bool IsKnownMatch(string expectedLanguage, string actualLanguage)
        {
            if (M2Language.IsChinese(expectedLanguage) && MatchesAny(actualLanguage, "zho", "zh", "chi", "zh-hans", "hans", "chs", "zh-cn", "zh-hant", "hant", "cht", "big5", "zh-tw", "zh-hk", "zh-mo"))
            {
                return true;
            }

            return string.Equals(expectedLanguage, actualLanguage, StringComparison.OrdinalIgnoreCase);
        }

        private static bool IsTextualFallbackMatch(string expectedLanguage, string text)
        {
            if (string.IsNullOrWhiteSpace(text) || !M2Language.IsChinese(expectedLanguage))
            {
                return false;
            }

            return text.IndexOf("中文", StringComparison.Ordinal) >= 0
                || text.IndexOf("简", StringComparison.Ordinal) >= 0
                || text.IndexOf("繁", StringComparison.Ordinal) >= 0
                || text.IndexOf("中英", StringComparison.Ordinal) >= 0;
        }

        private static string DetectStreamVariant(M2SubtitleStreamSnapshot stream)
        {
            var textual = BuildEvidence(stream).Trim();
            if (textual.Length == 0)
            {
                return null;
            }

            if (ContainsTag(textual, "zh-hans")
                || ContainsTag(textual, "hans")
                || ContainsTag(textual, "chs")
                || ContainsTag(textual, "sc")
                || ContainsTag(textual, "simplified")
                || MatchesAny(textual, "简体", "简"))
            {
                return "zh-Hans";
            }

            if (ContainsTag(textual, "zh-hant")
                || ContainsTag(textual, "hant")
                || ContainsTag(textual, "cht")
                || ContainsTag(textual, "big5")
                || ContainsTag(textual, "tc")
                || ContainsTag(textual, "traditional")
                || MatchesAny(textual, "繁体", "繁"))
            {
                return "zh-Hant";
            }

            return null;
        }

        private static bool MatchesAny(string value, params string[] aliases)
        {
            value = (value ?? string.Empty).ToLowerInvariant();
            foreach (var alias in aliases)
            {
                if (value == alias || (ContainsNonAscii(alias) && value.Contains(alias)))
                {
                    return true;
                }
            }

            return false;
        }

        private static bool ContainsNonAscii(string value)
        {
            foreach (var character in value)
            {
                if (character > 127)
                {
                    return true;
                }
            }

            return false;
        }

        private static string BuildEvidence(M2SubtitleStreamSnapshot stream)
        {
            return ((stream.Language ?? string.Empty) + " "
                + (stream.Title ?? string.Empty) + " "
                + GetFileName(stream.Path ?? string.Empty)).Trim();
        }

        private static string GetFileName(string path)
        {
            if (string.IsNullOrWhiteSpace(path))
            {
                return string.Empty;
            }

            var index = Math.Max(path.LastIndexOf('/'), path.LastIndexOf('\\'));
            return index >= 0 ? path.Substring(index + 1) : path;
        }

        private static bool ContainsTag(string value, string expectedTag)
        {
            if (string.IsNullOrWhiteSpace(value) || string.IsNullOrWhiteSpace(expectedTag))
            {
                return false;
            }

            var normalized = TagSeparator.Replace(value.ToLowerInvariant(), " ");
            return (" " + normalized + " ").IndexOf(" " + expectedTag.Trim().ToLowerInvariant() + " ", StringComparison.Ordinal) >= 0;
        }
    }
}
