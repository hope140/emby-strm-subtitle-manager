using System;
using System.Collections.Generic;

namespace SubSteward.Plugin.M1
{
    public sealed class M1Cue
    {
        public int Index { get; set; }

        public int StartMilliseconds { get; set; }

        public int EndMilliseconds { get; set; }

        public string Text { get; set; }

        public string StyleName { get; set; }
    }

    public sealed class M1ValidationResult
    {
        public string Format { get; set; }

        public string Encoding { get; set; }

        public string Health { get; set; }

        public bool HasReplacementCharacter { get; set; }

        public bool HasNulCharacter { get; set; }

        public bool HasIllegalControlCharacter { get; set; }

        public bool HasSrtNumberingIssue { get; set; }

        public bool HasAssOverrideTagIssue { get; set; }

        public List<string> Reasons { get; } = new List<string>();

        public List<M1Cue> Cues { get; } = new List<M1Cue>();

        public bool HasHanCharacters { get; set; }

        public bool IsUsable
        {
            get { return string.Equals(Health, "PASS", StringComparison.Ordinal) || string.Equals(Health, "WARNING", StringComparison.Ordinal); }
        }
    }

    public sealed class M1CandidateRecord
    {
        public string Token { get; set; }

        public Guid ItemId { get; set; }

        public string MediaSourceId { get; set; }

        public string Language { get; set; }

        public string RequestedLanguageVariant { get; set; }

        public string RawId { get; set; }

        public bool TitleMatch { get; set; }

        public bool HashMatch { get; set; }

        public DateTime ExpiresAtUtc { get; set; }
    }

    public sealed class M1ArtifactRecord
    {
        public string Token { get; set; }

        public Guid ItemId { get; set; }

        public string MediaSourceId { get; set; }

        public string Language { get; set; }

        public string RequestedLanguageVariant { get; set; }

        public byte[] Content { get; set; }

        public M1ValidationResult Validation { get; set; }

        public bool TitleMatch { get; set; }

        public bool HashMatch { get; set; }

        public DateTime ExpiresAtUtc { get; set; }
    }
}
