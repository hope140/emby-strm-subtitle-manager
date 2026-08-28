using System;
using System.Collections.Generic;
using System.Text;
using System.Text.RegularExpressions;

namespace SubSteward.Plugin.M1
{
    /// <summary>
    /// Applies one explicit, uniform timeline offset while preserving subtitle
    /// text, ASS metadata, source encoding, BOM, and newline style.
    /// </summary>
    public sealed class M1SubtitleTimelineShifter
    {
        public const int MaxAbsoluteOffsetMilliseconds = 10 * 60 * 1000;

        private static readonly Regex SrtTimeline = new Regex(
            @"^(?<leading>\s*)(?<start>\d{2}:\d{2}:\d{2}[,.]\d{3})(?<arrow>\s+-->\s+)(?<end>\d{2}:\d{2}:\d{2}[,.]\d{3})(?<trailing>.*)$",
            RegexOptions.CultureInvariant);

        public byte[] Shift(byte[] content, string format, int offsetMilliseconds)
        {
            if (content == null || content.Length == 0)
            {
                throw new ArgumentException("Subtitle content is empty.", nameof(content));
            }

            if (offsetMilliseconds == 0 || Math.Abs((long)offsetMilliseconds) > MaxAbsoluteOffsetMilliseconds)
            {
                throw new ArgumentOutOfRangeException(
                    nameof(offsetMilliseconds),
                    "Timeline offset must be non-zero and no more than 10 minutes in either direction.");
            }

            var normalizedFormat = NormalizeFormat(format);
            if (normalizedFormat != "srt" && normalizedFormat != "ass" && normalizedFormat != "ssa")
            {
                throw new NotSupportedException("Timeline shifting supports SRT, ASS, and SSA only.");
            }

            if ((normalizedFormat == "ass" || normalizedFormat == "ssa") && offsetMilliseconds % 10 != 0)
            {
                throw new ArgumentException("ASS and SSA offsets must use 10 millisecond increments.", nameof(offsetMilliseconds));
            }

            var decoded = Decode(content);
            var parts = Regex.Split(decoded.Text, "(\r\n|\n|\r)");
            var shiftedCount = normalizedFormat == "srt"
                ? ShiftSrt(parts, offsetMilliseconds)
                : ShiftAss(parts, offsetMilliseconds);
            if (shiftedCount == 0)
            {
                throw new InvalidOperationException("Subtitle contains no timeline entries that can be shifted.");
            }

            return Encode(string.Concat(parts), decoded.Encoding, decoded.Preamble);
        }

        private static int ShiftSrt(IList<string> parts, int offsetMilliseconds)
        {
            var shiftedCount = 0;
            for (var index = 0; index < parts.Count; index += 2)
            {
                var match = SrtTimeline.Match(parts[index]);
                if (!match.Success)
                {
                    continue;
                }

                var start = ParseSrtTime(match.Groups["start"].Value);
                var end = ParseSrtTime(match.Groups["end"].Value);
                var shiftedStart = ShiftTime(start, offsetMilliseconds);
                var shiftedEnd = ShiftTime(end, offsetMilliseconds);
                parts[index] = match.Groups["leading"].Value
                    + FormatSrtTime(shiftedStart, match.Groups["start"].Value[8])
                    + match.Groups["arrow"].Value
                    + FormatSrtTime(shiftedEnd, match.Groups["end"].Value[8])
                    + match.Groups["trailing"].Value;
                shiftedCount++;
            }

            return shiftedCount;
        }

        private static int ShiftAss(IList<string> parts, int offsetMilliseconds)
        {
            var inEvents = false;
            var startIndex = 1;
            var endIndex = 2;
            var fieldCount = 10;
            var shiftedCount = 0;
            for (var index = 0; index < parts.Count; index += 2)
            {
                var line = parts[index];
                var trimmed = line.Trim();
                if (trimmed.StartsWith("[", StringComparison.Ordinal) && trimmed.EndsWith("]", StringComparison.Ordinal))
                {
                    inEvents = trimmed.Equals("[Events]", StringComparison.OrdinalIgnoreCase);
                    continue;
                }

                if (!inEvents)
                {
                    continue;
                }

                if (trimmed.StartsWith("Format:", StringComparison.OrdinalIgnoreCase))
                {
                    var formatFields = trimmed.Substring("Format:".Length).Split(',');
                    startIndex = FindField(formatFields, "Start");
                    endIndex = FindField(formatFields, "End");
                    fieldCount = formatFields.Length;
                    if (startIndex < 0 || endIndex < 0)
                    {
                        throw new InvalidOperationException("ASS Events format is missing Start or End.");
                    }

                    continue;
                }

                var dialogueIndex = line.IndexOf("Dialogue:", StringComparison.OrdinalIgnoreCase);
                if (dialogueIndex < 0 || !string.IsNullOrWhiteSpace(line.Substring(0, dialogueIndex)))
                {
                    continue;
                }

                var prefixLength = dialogueIndex + "Dialogue:".Length;
                var fieldsValue = line.Substring(prefixLength);
                var dialogueFields = fieldsValue.Split(new[] { ',' }, fieldCount);
                if (dialogueFields.Length < fieldCount || startIndex >= dialogueFields.Length || endIndex >= dialogueFields.Length)
                {
                    throw new InvalidOperationException("ASS dialogue has too few fields for its Events format.");
                }

                var start = ParseAssTime(dialogueFields[startIndex].Trim());
                var end = ParseAssTime(dialogueFields[endIndex].Trim());
                dialogueFields[startIndex] = ReplaceTrimmed(dialogueFields[startIndex], FormatAssTime(ShiftTime(start, offsetMilliseconds)));
                dialogueFields[endIndex] = ReplaceTrimmed(dialogueFields[endIndex], FormatAssTime(ShiftTime(end, offsetMilliseconds)));
                parts[index] = line.Substring(0, prefixLength) + string.Join(",", dialogueFields);
                shiftedCount++;
            }

            return shiftedCount;
        }

        private static int ShiftTime(int milliseconds, int offsetMilliseconds)
        {
            var shifted = (long)milliseconds + offsetMilliseconds;
            if (shifted < 0 || shifted > int.MaxValue)
            {
                throw new InvalidOperationException("Timeline offset would move a subtitle cue outside the supported time range.");
            }

            return (int)shifted;
        }

        private static int ParseSrtTime(string value)
        {
            var parts = value.Replace(',', ':').Replace('.', ':').Split(':');
            int hours;
            int minutes;
            int seconds;
            int milliseconds;
            if (parts.Length != 4
                || !int.TryParse(parts[0], out hours)
                || !int.TryParse(parts[1], out minutes)
                || !int.TryParse(parts[2], out seconds)
                || !int.TryParse(parts[3], out milliseconds)
                || minutes > 59
                || seconds > 59
                || milliseconds > 999)
            {
                throw new InvalidOperationException("SRT cue has an invalid timestamp.");
            }

            return checked((((hours * 60) + minutes) * 60 + seconds) * 1000 + milliseconds);
        }

        private static string FormatSrtTime(int milliseconds, char separator)
        {
            var hours = milliseconds / 3600000;
            if (hours > 99)
            {
                throw new InvalidOperationException("Shifted SRT timestamp exceeds the supported two-digit hour range.");
            }

            var minutes = milliseconds / 60000 % 60;
            var seconds = milliseconds / 1000 % 60;
            var millis = milliseconds % 1000;
            return hours.ToString("00") + ":" + minutes.ToString("00") + ":" + seconds.ToString("00") + separator + millis.ToString("000");
        }

        private static int ParseAssTime(string value)
        {
            var colon = value.Split(':');
            if (colon.Length != 3)
            {
                throw new InvalidOperationException("ASS dialogue has an invalid timestamp.");
            }

            var secondParts = colon[2].Split('.');
            int hours;
            int minutes;
            int seconds;
            int centiseconds;
            if (secondParts.Length != 2
                || !int.TryParse(colon[0], out hours)
                || !int.TryParse(colon[1], out minutes)
                || !int.TryParse(secondParts[0], out seconds)
                || !int.TryParse(secondParts[1], out centiseconds)
                || minutes > 59
                || seconds > 59
                || centiseconds > 99)
            {
                throw new InvalidOperationException("ASS dialogue has an invalid timestamp.");
            }

            return checked((((hours * 60) + minutes) * 60 + seconds) * 1000 + centiseconds * 10);
        }

        private static string FormatAssTime(int milliseconds)
        {
            var hours = milliseconds / 3600000;
            var minutes = milliseconds / 60000 % 60;
            var seconds = milliseconds / 1000 % 60;
            var centiseconds = milliseconds % 1000 / 10;
            return hours + ":" + minutes.ToString("00") + ":" + seconds.ToString("00") + "." + centiseconds.ToString("00");
        }

        private static int FindField(string[] fields, string expected)
        {
            for (var index = 0; index < fields.Length; index++)
            {
                if (string.Equals(fields[index].Trim(), expected, StringComparison.OrdinalIgnoreCase))
                {
                    return index;
                }
            }

            return -1;
        }

        private static string ReplaceTrimmed(string original, string replacement)
        {
            var first = 0;
            while (first < original.Length && char.IsWhiteSpace(original[first]))
            {
                first++;
            }

            var last = original.Length - 1;
            while (last >= first && char.IsWhiteSpace(original[last]))
            {
                last--;
            }

            return original.Substring(0, first) + replacement + original.Substring(last + 1);
        }

        private static DecodedSubtitle Decode(byte[] content)
        {
            Encoding encoding;
            byte[] preamble;
            var offset = 0;
            if (content.Length >= 3 && content[0] == 0xEF && content[1] == 0xBB && content[2] == 0xBF)
            {
                encoding = new UTF8Encoding(false, true);
                preamble = new byte[] { 0xEF, 0xBB, 0xBF };
                offset = 3;
            }
            else if (content.Length >= 2 && content[0] == 0xFF && content[1] == 0xFE)
            {
                encoding = new UnicodeEncoding(false, false, true);
                preamble = new byte[] { 0xFF, 0xFE };
                offset = 2;
            }
            else if (content.Length >= 2 && content[0] == 0xFE && content[1] == 0xFF)
            {
                encoding = new UnicodeEncoding(true, false, true);
                preamble = new byte[] { 0xFE, 0xFF };
                offset = 2;
            }
            else
            {
                encoding = new UTF8Encoding(false, true);
                preamble = new byte[0];
            }

            try
            {
                return new DecodedSubtitle
                {
                    Text = encoding.GetString(content, offset, content.Length - offset),
                    Encoding = encoding,
                    Preamble = preamble
                };
            }
            catch (DecoderFallbackException exception)
            {
                throw new InvalidOperationException("Timeline shifting requires a losslessly decoded subtitle.", exception);
            }
        }

        private static byte[] Encode(string text, Encoding encoding, byte[] preamble)
        {
            var body = encoding.GetBytes(text);
            if (preamble.Length == 0)
            {
                return body;
            }

            var result = new byte[preamble.Length + body.Length];
            Buffer.BlockCopy(preamble, 0, result, 0, preamble.Length);
            Buffer.BlockCopy(body, 0, result, preamble.Length, body.Length);
            return result;
        }

        private static string NormalizeFormat(string format)
        {
            var value = (format ?? string.Empty).Trim().TrimStart('.').ToLowerInvariant();
            return value == "subrip" ? "srt" : value;
        }

        private sealed class DecodedSubtitle
        {
            public string Text { get; set; }

            public Encoding Encoding { get; set; }

            public byte[] Preamble { get; set; }
        }
    }
}
