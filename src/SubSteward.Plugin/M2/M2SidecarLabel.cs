using System;
using SubSteward.Plugin.M1;

namespace SubSteward.Plugin.M2
{
    public static class M2SidecarLabel
    {
        public static string Build(M1ValidationResult validation, string requestedLanguageVariant)
        {
            if (validation == null)
            {
                throw new ArgumentNullException(nameof(validation));
            }

            if (!string.Equals(validation.Health, "FAIL", StringComparison.Ordinal))
            {
                var report = new M2QualityAnalyzer().Analyze(validation);
                var secondary = BilingualSuffix(report);

                if (!string.IsNullOrWhiteSpace(secondary))
                {
                    return secondary == "日" ? "中日双语" : "中英双语";
                }

                if (requestedLanguageVariant == "zh-Hans")
                {
                    return "中文简体";
                }

                if (requestedLanguageVariant == "zh-Hant")
                {
                    return "中文繁體";
                }

            }

            return null;
        }

        private static string BilingualSuffix(M2QualityReport report)
        {
            if (report.CueCount == 0 || !report.BilingualDetected)
            {
                return null;
            }

            var threshold = Math.Max(1, (int)Math.Ceiling(report.CueCount * 0.05d));
            var bilingualCueCount = Math.Max(report.BilingualCueCount, report.JapaneseCueCount);
            if (bilingualCueCount < threshold)
            {
                return null;
            }

            if (report.JapaneseCueCount >= threshold)
            {
                return "日";
            }

            return report.SecondaryLanguageCueCount >= threshold ? "英" : null;
        }
    }
}
