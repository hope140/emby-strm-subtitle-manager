using System;
using System.Text;
using SubSteward.Plugin.M1;

namespace SubSteward.Plugin.M2
{
    public sealed class M2QualityAnalyzer
    {
        private const string DefaultTargetLanguage = "zh-Hans";
        private const string DefaultSecondaryLanguage = "eng";

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
            report.BilingualDetected = report.TargetLanguagePresent
                && (report.SecondaryLanguagePresent || (report.JapaneseCueCount > 0 && !M2Language.IsJapanese(normalizedTarget)));
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
            var runLength = 0;
            foreach (var character in text)
            {
                if ((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z'))
                {
                    runLength++;
                    if (runLength >= 2)
                    {
                        return true;
                    }
                }
                else
                {
                    runLength = 0;
                }
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
