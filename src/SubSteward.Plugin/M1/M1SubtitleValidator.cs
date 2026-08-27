using System;
using System.Collections.Generic;
using System.IO;
using System.Text;
using System.Text.RegularExpressions;

namespace SubSteward.Plugin.M1
{
    /// <summary>
    /// Small M1 validator. It checks structure and timing without rewriting
    /// subtitle content or interpreting ASS effects.
    /// </summary>
    public sealed class M1SubtitleValidator
    {
        private const int MaxBytes = 16 * 1024 * 1024;
        private static readonly Regex SrtTimestamp = new Regex(
            @"^(?<start>\d{2}:\d{2}:\d{2}[,.]\d{3})\s+-->\s+(?<end>\d{2}:\d{2}:\d{2}[,.]\d{3})",
            RegexOptions.CultureInvariant);
        private static readonly Regex AssTimestamp = new Regex(
            @"^(?<hours>\d+):(?<minutes>\d{2}):(?<seconds>\d{2})\.(?<centiseconds>\d{2})$",
            RegexOptions.CultureInvariant);

        public M1ValidationResult Validate(byte[] content, string format)
        {
            if (content == null || content.Length == 0)
            {
                return Failure(format, "subtitle content is empty");
            }

            if (content.Length > MaxBytes)
            {
                return Failure(format, "subtitle content exceeds the M1 size limit");
            }

            var result = new M1ValidationResult
            {
                Format = NormalizeFormat(format)
            };
            bool lossy;
            string text;
            try
            {
                text = Decode(content, result, out lossy);
            }
            catch (DecoderFallbackException)
            {
                return Failure(result.Format, "subtitle encoding is invalid");
            }

            if (lossy)
            {
                result.Health = "WARNING";
                result.Reasons.Add("subtitle encoding required replacement characters");
            }

            result.HasNulCharacter = TextHasNulCharacters(text);
            result.HasIllegalControlCharacter = !result.HasNulCharacter && TextHasIllegalControlCharacters(text);
            result.HasReplacementCharacter = lossy || TextHasReplacementCharacters(text);

            if (result.HasNulCharacter)
            {
                result.Health = "WARNING";
                result.Reasons.Add("subtitle contains a NUL control character");
            }

            if (result.HasIllegalControlCharacter)
            {
                result.Health = "WARNING";
                result.Reasons.Add("subtitle contains an unexpected control character");
            }

            if (result.Format == "srt")
            {
                ValidateSrt(text, result);
            }
            else if (result.Format == "ass" || result.Format == "ssa")
            {
                ValidateAss(text, result);
            }
            else
            {
                result.Health = "FAIL";
                result.Reasons.Add("subtitle format is unsupported");
            }

            if (result.Health == null)
            {
                result.Health = "PASS";
            }

            result.HasHanCharacters = HasHanCharacters(result);

            return result;
        }

        public static bool HasHanCharacters(M1ValidationResult result)
        {
            foreach (var cue in result.Cues)
            {
                foreach (var character in cue.Text ?? string.Empty)
                {
                    if (character >= '\u4e00' && character <= '\u9fff')
                    {
                        return true;
                    }
                }
            }

            return false;
        }

        private static string Decode(byte[] content, M1ValidationResult result, out bool lossy)
        {
            lossy = false;
            Encoding encoding;
            int offset;
            if (content.Length >= 3 && content[0] == 0xEF && content[1] == 0xBB && content[2] == 0xBF)
            {
                encoding = new UTF8Encoding(false, true);
                offset = 3;
                result.Encoding = "UTF-8 BOM";
            }
            else if (content.Length >= 2 && content[0] == 0xFF && content[1] == 0xFE)
            {
                encoding = new UnicodeEncoding(false, false, true);
                offset = 2;
                result.Encoding = "UTF-16 LE";
            }
            else if (content.Length >= 2 && content[0] == 0xFE && content[1] == 0xFF)
            {
                encoding = new UnicodeEncoding(true, false, true);
                offset = 2;
                result.Encoding = "UTF-16 BE";
            }
            else
            {
                encoding = new UTF8Encoding(false, true);
                offset = 0;
                result.Encoding = "UTF-8";
            }

            try
            {
                return encoding.GetString(content, offset, content.Length - offset).Replace("\r\n", "\n").Replace('\r', '\n');
            }
            catch (DecoderFallbackException)
            {
                lossy = true;
                result.Encoding = "unknown (replacement)";
                return new UTF8Encoding(false, false).GetString(content).Replace("\r\n", "\n").Replace('\r', '\n');
            }
        }

        private static void ValidateSrt(string text, M1ValidationResult result)
        {
            var lines = text.Split(new[] { '\n' }, StringSplitOptions.None);
            var position = 0;
            var cueNumber = 1;
            while (position < lines.Length)
            {
                while (position < lines.Length && string.IsNullOrWhiteSpace(lines[position]))
                {
                    position++;
                }

                if (position >= lines.Length)
                {
                    break;
                }

                var index = cueNumber;
                int parsedIndex;
                if (int.TryParse(lines[position].Trim(), out parsedIndex))
                {
                    index = parsedIndex;
                    position++;
                    if (parsedIndex != cueNumber && !result.HasSrtNumberingIssue)
                    {
                        result.HasSrtNumberingIssue = true;
                        if (!string.Equals(result.Health, "FAIL", StringComparison.Ordinal))
                        {
                            result.Health = "WARNING";
                            result.Reasons.Add("SRT cue numbering is inconsistent");
                        }
                    }
                }

                if (position >= lines.Length)
                {
                    result.Health = "FAIL";
                    result.Reasons.Add("SRT cue is missing its timestamp");
                    return;
                }

                var match = SrtTimestamp.Match(lines[position].Trim());
                if (!match.Success)
                {
                    result.Health = "FAIL";
                    result.Reasons.Add("SRT cue has an invalid timestamp");
                    return;
                }

                int start;
                int end;
                if (!TryParseSrtTime(match.Groups["start"].Value, out start) || !TryParseSrtTime(match.Groups["end"].Value, out end) || start >= end)
                {
                    result.Health = "FAIL";
                    result.Reasons.Add("SRT cue has an invalid timeline");
                    return;
                }

                position++;
                var textLines = new List<string>();
                while (position < lines.Length && !string.IsNullOrWhiteSpace(lines[position]))
                {
                    textLines.Add(lines[position]);
                    position++;
                }

                if (textLines.Count == 0)
                {
                    result.Health = "FAIL";
                    result.Reasons.Add("SRT cue has no text");
                    return;
                }

                result.Cues.Add(new M1Cue
                {
                    Index = index,
                    StartMilliseconds = start,
                    EndMilliseconds = end,
                    Text = string.Join("\n", textLines)
                });
                cueNumber++;
            }

            if (result.Cues.Count == 0)
            {
                result.Health = "FAIL";
                result.Reasons.Add("SRT contains no cues");
            }
        }

        private static void ValidateAss(string text, M1ValidationResult result)
        {
            var hasEvents = false;
            var startIndex = 1;
            var endIndex = 2;
            var styleIndex = 3;
            var textIndex = 9;
            var fieldCount = 10;
            var lines = text.Split(new[] { '\n' }, StringSplitOptions.None);
            foreach (var rawLine in lines)
            {
                var line = rawLine.TrimEnd();
                if (line.Equals("[Events]", StringComparison.OrdinalIgnoreCase))
                {
                    hasEvents = true;
                    continue;
                }

                if (hasEvents && line.StartsWith("Format:", StringComparison.OrdinalIgnoreCase))
                {
                    var formatFields = line.Substring("Format:".Length).Split(',');
                    var parsedStartIndex = FindAssField(formatFields, "Start");
                    var parsedEndIndex = FindAssField(formatFields, "End");
                    var parsedStyleIndex = FindAssField(formatFields, "Style");
                    var parsedTextIndex = FindAssField(formatFields, "Text");
                    if (parsedStartIndex < 0 || parsedEndIndex < 0 || parsedTextIndex < 0)
                    {
                        result.Health = "FAIL";
                        result.Reasons.Add("ASS Events format is missing Start, End, or Text");
                        return;
                    }

                    startIndex = parsedStartIndex;
                    endIndex = parsedEndIndex;
                    styleIndex = parsedStyleIndex;
                    textIndex = parsedTextIndex;
                    fieldCount = formatFields.Length;
                    continue;
                }

                if (!line.StartsWith("Dialogue:", StringComparison.OrdinalIgnoreCase))
                {
                    continue;
                }

                var fields = line.Substring("Dialogue:".Length).TrimStart().Split(new[] { ',' }, fieldCount);
                if (fields.Length < fieldCount)
                {
                    result.Health = "FAIL";
                    result.Reasons.Add("ASS dialogue has too few fields");
                    return;
                }

                int start;
                int end;
                if (!TryParseAssTime(fields[startIndex].Trim(), out start) || !TryParseAssTime(fields[endIndex].Trim(), out end) || start >= end)
                {
                    result.Health = "FAIL";
                    result.Reasons.Add("ASS dialogue has an invalid timeline");
                    return;
                }

                result.Cues.Add(new M1Cue
                {
                    Index = result.Cues.Count + 1,
                    StartMilliseconds = start,
                    EndMilliseconds = end,
                    Text = fields[textIndex],
                    StyleName = styleIndex >= 0 && styleIndex < fields.Length ? fields[styleIndex].Trim() : null
                });

                if (!result.HasAssOverrideTagIssue && !OverrideTagsAreBalanced(fields[textIndex]))
                {
                    result.HasAssOverrideTagIssue = true;
                    if (!string.Equals(result.Health, "FAIL", StringComparison.Ordinal))
                    {
                        result.Health = "WARNING";
                        result.Reasons.Add("ASS dialogue has an unbalanced override tag");
                    }
                }
            }

            if (!hasEvents || result.Cues.Count == 0)
            {
                result.Health = "FAIL";
                result.Reasons.Add(!hasEvents ? "ASS is missing an Events section" : "ASS contains no dialogue cues");
            }
        }

        private static bool TextHasNulCharacters(string text)
        {
            return text.IndexOf('\0') >= 0;
        }

        private static bool TextHasReplacementCharacters(string text)
        {
            return text.IndexOf('\ufffd') >= 0;
        }

        private static bool TextHasIllegalControlCharacters(string text)
        {
            foreach (var character in text)
            {
                if (character != '\n' && character != '\t' && char.IsControl(character))
                {
                    return true;
                }
            }

            return false;
        }

        private static bool OverrideTagsAreBalanced(string text)
        {
            foreach (var line in text.Split('\n'))
            {
                var depth = 0;
                for (var index = 0; index < line.Length; index++)
                {
                    if (line[index] == '\\')
                    {
                        index++;
                    }
                    else if (line[index] == '{')
                    {
                        depth++;
                    }
                    else if (line[index] == '}')
                    {
                        depth--;
                        if (depth < 0)
                        {
                            return false;
                        }
                    }
                }

                if (depth != 0)
                {
                    return false;
                }
            }

            return true;
        }

        private static int FindAssField(string[] fields, string expectedName)
        {
            for (var index = 0; index < fields.Length; index++)
            {
                if (string.Equals(fields[index].Trim(), expectedName, StringComparison.OrdinalIgnoreCase))
                {
                    return index;
                }
            }

            return -1;
        }

        private static bool TryParseSrtTime(string value, out int milliseconds)
        {
            milliseconds = 0;
            var normalized = value.Replace(',', ':').Replace('.', ':');
            var parts = normalized.Split(':');
            if (parts.Length != 4)
            {
                return false;
            }

            int hours;
            int minutes;
            int seconds;
            int millis;
            if (!int.TryParse(parts[0], out hours) || !int.TryParse(parts[1], out minutes) || !int.TryParse(parts[2], out seconds) || !int.TryParse(parts[3], out millis))
            {
                return false;
            }

            if (minutes > 59 || seconds > 59 || millis > 999)
            {
                return false;
            }

            milliseconds = (((hours * 60) + minutes) * 60 + seconds) * 1000 + millis;
            return true;
        }

        private static bool TryParseAssTime(string value, out int milliseconds)
        {
            milliseconds = 0;
            var match = AssTimestamp.Match(value);
            if (!match.Success)
            {
                return false;
            }

            int hours;
            int minutes;
            int seconds;
            int centiseconds;
            if (!int.TryParse(match.Groups["hours"].Value, out hours) || !int.TryParse(match.Groups["minutes"].Value, out minutes) || !int.TryParse(match.Groups["seconds"].Value, out seconds) || !int.TryParse(match.Groups["centiseconds"].Value, out centiseconds))
            {
                return false;
            }

            if (minutes > 59 || seconds > 59 || centiseconds > 99)
            {
                return false;
            }

            milliseconds = (((hours * 60) + minutes) * 60 + seconds) * 1000 + centiseconds * 10;
            return true;
        }

        private static string NormalizeFormat(string format)
        {
            var value = (format ?? string.Empty).Trim().ToLowerInvariant();
            if (value == "subrip")
            {
                return "srt";
            }

            return value.TrimStart('.');
        }

        private static M1ValidationResult Failure(string format, string reason)
        {
            var result = new M1ValidationResult
            {
                Format = NormalizeFormat(format),
                Health = "FAIL"
            };
            result.Reasons.Add(reason);
            return result;
        }
    }
}
