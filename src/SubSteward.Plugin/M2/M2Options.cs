using System;
using System.Collections.Generic;

namespace SubSteward.Plugin.M2
{
    public static class M2Options
    {
        private const string DefaultTargetLanguage = "zh-Hans";
        private const string DefaultSecondaryLanguage = "eng";
        private const string DefaultFormatOrder = "ass,ssa,srt";

        public static M2PreferenceOptions ParsePreferenceOptions(bool preferBilingual, string formatOrder)
        {
            var formats = new List<string>();
            if (!string.IsNullOrWhiteSpace(formatOrder))
            {
                foreach (var value in formatOrder.Split(new[] { ',', ';' }, StringSplitOptions.RemoveEmptyEntries))
                {
                    var format = value.Trim().TrimStart('.').ToLowerInvariant();
                    if (format == "subrip")
                    {
                        format = "srt";
                    }

                    if (format.Length > 0 && !formats.Contains(format))
                    {
                        formats.Add(format);
                    }
                }
            }

            return new M2PreferenceOptions
            {
                PreferBilingual = preferBilingual,
                FormatOrder = formats.Count > 0 ? formats.ToArray() : ParseFormatOrder(DefaultFormatOrder)
            };
        }

        public static string[] ParseFormatOrder(string formatOrder)
        {
            return ParsePreferenceOptions(false, formatOrder).FormatOrder;
        }

        public static string ResolveSubtitleLanguageTag(string requestedLanguageVariant, string fallbackCode)
        {
            var selection = M2Language.Parse(requestedLanguageVariant, fallbackCode);
            var variant = (selection.Variant ?? string.Empty).Trim().ToLowerInvariant();
            if (variant == "zh-hans")
            {
                return "zh-CN";
            }

            if (variant == "zh-hant")
            {
                return "zh-TW";
            }

            return selection.Code;
        }

        public static string CanonicalizeTargetLanguage(string language)
        {
            var selection = ParseTargetLanguage(language);
            return selection.Variant ?? selection.Code;
        }

        public static string CanonicalizeSecondaryLanguage(string language)
        {
            var selection = ParseSecondaryLanguage(language);
            return selection.Variant ?? selection.Code;
        }

        public static string NormalizeTargetLanguage(string language)
        {
            return ParseTargetLanguage(language).Code;
        }

        public static string NormalizeSecondaryLanguage(string language)
        {
            return ParseSecondaryLanguage(language).Code;
        }

        public static M2LanguageSelection ParseTargetLanguage(string language)
        {
            return M2Language.Parse(language, DefaultTargetLanguage);
        }

        public static M2LanguageSelection ParseSecondaryLanguage(string language)
        {
            return M2Language.Parse(language, DefaultSecondaryLanguage);
        }
    }
}
