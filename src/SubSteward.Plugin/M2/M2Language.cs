using System;

namespace SubSteward.Plugin.M2
{
    public sealed class M2LanguageSelection
    {
        public string Code { get; set; }

        public string Variant { get; set; }

        public string Label { get; set; }
    }

    public static class M2Language
    {
        public const string Chinese = "zho";
        public const string English = "eng";

        public static M2LanguageSelection Parse(string language, string fallbackCode)
        {
            var value = (language ?? string.Empty).Trim().ToLowerInvariant();
            if (value.Length == 0)
            {
                var fallback = (fallbackCode ?? string.Empty).Trim();
                return fallback.Length == 0
                    ? new M2LanguageSelection { Code = string.Empty, Label = string.Empty }
                    : Parse(fallback, null);
            }

            if (MatchesAny(value, "zho", "zh", "chi"))
            {
                return new M2LanguageSelection { Code = Chinese, Label = "中文" };
            }

            if (MatchesAny(value, "zh-hans", "hans", "chs", "gb", "gbk", "sc", "simplified", "simplified-chinese", "zh-cn", "zh-sg", "zh-my"))
            {
                return new M2LanguageSelection { Code = Chinese, Variant = "zh-Hans", Label = "中文（简体）" };
            }

            if (MatchesAny(value, "zh-hant", "hant", "cht", "big5", "tc", "traditional", "traditional-chinese", "zh-tw", "zh-hk", "zh-mo"))
            {
                return new M2LanguageSelection { Code = Chinese, Variant = "zh-Hant", Label = "中文（繁体）" };
            }

            if (MatchesAny(value, "eng", "en", "english"))
            {
                return new M2LanguageSelection { Code = English, Label = "English" };
            }

            var normalizedValue = value.Replace("_", "-");
            return new M2LanguageSelection { Code = normalizedValue, Label = normalizedValue };
        }

        public static bool IsChinese(string code)
        {
            return MatchesAny((code ?? string.Empty).Trim(),
                "zho", "zh", "chi",
                "zh-hans", "hans", "chs", "gb", "gbk", "sc", "simplified", "simplified-chinese",
                "zh-cn", "zh-sg", "zh-my",
                "zh-hant", "hant", "cht", "big5", "tc", "traditional", "traditional-chinese",
                "zh-tw", "zh-hk", "zh-mo");
        }

        public static bool IsEnglish(string code)
        {
            return MatchesAny((code ?? string.Empty).Trim(), "eng", "en");
        }

        public static bool IsJapanese(string code)
        {
            return MatchesAny((code ?? string.Empty).Trim(), "jpn", "ja", "japanese");
        }

        public static bool StreamMatchesVariant(M2SubtitleStreamSnapshot stream, string variant)
        {
            if (stream == null || string.IsNullOrWhiteSpace(variant))
            {
                return true;
            }

            var textual = ((stream.Language ?? string.Empty) + " " + (stream.Title ?? string.Empty) + " " + (stream.Path ?? string.Empty)).ToLowerInvariant();
            if (variant == "zh-Hans")
            {
                return !ContainsEvidence(textual, "zh-hant", "hant", "cht", "big5", "zh-tw", "zh-hk", "zh-mo", "繁体", "繁", "雙繁");
            }

            if (variant == "zh-Hant")
            {
                return !ContainsEvidence(textual, "zh-hans", "hans", "chs", "zh-cn", "zh-sg", "zh-my", "简体", "简", "双简");
            }

            return true;
        }

        private static bool ContainsEvidence(string value, params string[] aliases)
        {
            foreach (var alias in aliases)
            {
                if (value.IndexOf(alias, StringComparison.OrdinalIgnoreCase) >= 0)
                {
                    return true;
                }
            }

            return false;
        }

        private static bool MatchesAny(string value, params string[] aliases)
        {
            foreach (var alias in aliases)
            {
                if (string.Equals(value, alias, StringComparison.OrdinalIgnoreCase))
                {
                    return true;
                }

                if (ContainsNonAscii(alias) && value.IndexOf(alias, StringComparison.OrdinalIgnoreCase) >= 0)
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
    }
}
