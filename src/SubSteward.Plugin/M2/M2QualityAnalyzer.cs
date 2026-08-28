using System;
using System.Collections.Generic;
using System.Linq;
using System.Text;
using System.Text.RegularExpressions;
using SubSteward.Plugin.M1;

namespace SubSteward.Plugin.M2
{
    public sealed class M2QualityAnalyzer
    {
        private const string DefaultTargetLanguage = "zh-Hans";
        private const string DefaultSecondaryLanguage = "eng";
        private static readonly Regex LatinWord = new Regex("[A-Za-z]{2,}", RegexOptions.CultureInvariant);
        private static readonly HashSet<string> CommonEnglishWords = new HashSet<string>(StringComparer.OrdinalIgnoreCase)
        {
            "about", "again", "all", "am", "and", "are", "around", "as", "at", "away", "be", "because", "been", "before", "but", "can", "come", "could", "did", "do", "does", "done", "for", "from", "get", "give", "go", "good", "got", "have", "he", "hello", "her", "here", "him", "his", "how", "if", "in", "is", "it", "just", "know", "like", "look", "love", "make", "me", "more", "most", "my", "never", "no", "not", "now", "of", "oh", "on", "one", "or", "our", "out", "please", "really", "right", "say", "see", "she", "so", "some", "sorry", "stop", "thank", "thanks", "that", "the", "their", "them", "then", "there", "they", "this", "time", "to", "too", "very", "wait", "want", "was", "we", "well", "what", "when", "where", "who", "why", "will", "with", "would", "yes", "you", "your"
        };
        private static readonly HashSet<string> LatinNoiseTokens = new HashSet<string>(StringComparer.OrdinalIgnoreCase)
        {
            "aa", "aaa", "aac", "ass", "av", "awww", "bilibili", "bgm", "cmct", "com", "cry", "cut", "dts", "dvd", "hd", "html", "hdtv", "https", "http", "jj", "mkv", "neil", "nf", "qaq", "qq", "qwq", "rip", "srt", "tv", "up", "web", "www", "x264", "yoyo"
        };

        public M2QualityReport Analyze(M1ValidationResult validation, string targetLanguage = DefaultTargetLanguage, string secondaryLanguage = DefaultSecondaryLanguage)
        {
            if (validation == null)
            {
                throw new ArgumentNullException(nameof(validation));
            }

            var normalizedTarget = NormalizeLanguage(targetLanguage, DefaultTargetLanguage);
            var normalizedSecondary = NormalizeLanguage(secondaryLanguage, DefaultSecondaryLanguage);
            var secondaryActive = !string.Equals(normalizedTarget, normalizedSecondary, StringComparison.OrdinalIgnoreCase);
            var report = new M2QualityReport
            {
                TargetLanguage = normalizedTarget,
                SecondaryLanguage = secondaryActive ? normalizedSecondary : null,
                CueCount = validation.Cues.Count,
                Format = validation.Format,
                Encoding = validation.Encoding,
                Health = validation.Health ?? "PASS"
            };

            foreach (var cue in validation.Cues)
            {
                var visibleText = StripAssOverrideTags(cue.Text);
                var hasTargetLanguage = DetectLanguage(report.TargetLanguage, visibleText);
                var hasSecondaryLanguage = secondaryActive && DetectLanguage(normalizedSecondary, visibleText);
                var hasJapaneseText = HasKanaCharacter(visibleText);

                if (hasTargetLanguage)
                {
                    report.TargetLanguageCueCount++;
                }

                if (hasSecondaryLanguage)
                {
                    report.SecondaryLanguageCueCount++;
                }

                if (hasJapaneseText)
                {
                    report.JapaneseCueCount++;
                }

                if (hasTargetLanguage && hasSecondaryLanguage)
                {
                    report.BilingualCueCount++;
                }

                if (!string.IsNullOrWhiteSpace(cue.StyleName) && !string.Equals(cue.StyleName, "Default", StringComparison.OrdinalIgnoreCase))
                {
                    report.StyledCueCount++;
                }

                if (cue.Text != null && cue.Text.IndexOf("{\\", StringComparison.Ordinal) >= 0)
                {
                    report.EffectCueCount++;
                }
            }

            report.TargetLanguagePresent = report.TargetLanguageCueCount > 0;
            report.TargetLanguageConfidence = report.CueCount == 0 ? 0d : Math.Min(0.99d, report.TargetLanguageCueCount / (double)report.CueCount);
            report.SecondaryLanguagePresent = report.SecondaryLanguageCueCount > 0;
            // Bilingual means the configured target and secondary languages
            // are both evidenced in the text. Japanese is tracked separately
            // for the sidecar label and must not become English evidence.
            report.BilingualDetected = report.TargetLanguagePresent && report.SecondaryLanguagePresent;
            report.BilingualConfidence = report.CueCount == 0 ? 0d : Math.Min(0.99d, report.BilingualCueCount / (double)report.CueCount);

            if (report.EffectCueCount == 0)
            {
                report.EffectStrength = "None";
            }
            else if (report.EffectCueCount >= 10 && report.EffectCueCount * 5 >= report.CueCount)
            {
                report.EffectStrength = "Heavy";
            }
            else
            {
                report.EffectStrength = "Light";
            }

            return report;
        }

        private static string StripAssOverrideTags(string text)
        {
            if (string.IsNullOrEmpty(text))
            {
                return string.Empty;
            }

            var output = new StringBuilder(text.Length);
            for (var index = 0; index < text.Length; index++)
            {
                if (text[index] == '{')
                {
                    var closingIndex = text.IndexOf('}', index + 1);
                    if (closingIndex < 0)
                    {
                        output.Append(text[index]);
                        continue;
                    }

                    index = closingIndex;
                    continue;
                }

                output.Append(text[index]);
            }

            return output.ToString();
        }

        private static bool HasHanCharacter(string text)
        {
            foreach (var character in text)
            {
                if ((character >= '\u3400' && character <= '\u4dbf')
                    || (character >= '\u4e00' && character <= '\u9fff')
                    || (character >= '\uf900' && character <= '\ufaff'))
                {
                    return true;
                }
            }

            return false;
        }

        private static bool HasLatinCharacter(string text)
        {
            var meaningfulWords = new List<string>();
            foreach (Match match in LatinWord.Matches(text ?? string.Empty))
            {
                var word = match.Value;
                if (IsLatinNoiseToken(word))
                {
                    continue;
                }

                if (word.Length < 3)
                {
                    continue;
                }

                meaningfulWords.Add(word);
            }

            if (meaningfulWords.Count == 0)
            {
                return false;
            }

            if (meaningfulWords.Any(CommonEnglishWords.Contains))
            {
                return true;
            }

            // A pair of non-noise words is enough for a likely English phrase
            // such as "Spirited Away", but one isolated name or acronym is not.
            return meaningfulWords.Count >= 2;
        }

        private static bool IsLatinNoiseToken(string word)
        {
            if (LatinNoiseTokens.Contains(word))
            {
                return true;
            }

            var normalized = word.ToLowerInvariant();
            if (normalized.Length >= 4 && normalized[0] == 'a')
            {
                var allW = true;
                for (var index = 1; index < normalized.Length; index++)
                {
                    if (normalized[index] != 'w')
                    {
                        allW = false;
                        break;
                    }
                }

                if (allW)
                {
                    return true;
                }
            }

            if (word.Length >= 3 && word.ToUpperInvariant() == word && !CommonEnglishWords.Contains(word))
            {
                return true;
            }

            return false;
        }

        private static bool DetectLanguage(string language, string text)
        {
            if (M2Language.IsChinese(language))
            {
                return HasHanCharacter(text);
            }

            if (M2Language.IsEnglish(language))
            {
                return HasLatinCharacter(text);
            }

            if (M2Language.IsJapanese(language))
            {
                return HasKanaCharacter(text);
            }

            return false;
        }

        private static bool HasKanaCharacter(string text)
        {
            foreach (var character in text)
            {
                if ((character >= '\u3040' && character <= '\u309f')
                    || (character >= '\u30a0' && character <= '\u30ff'))
                {
                    return true;
                }
            }

            return false;
        }

        private static string NormalizeLanguage(string language, string defaultLanguage)
        {
            return M2Language.Parse(language, defaultLanguage).Code;
        }
    }
}
