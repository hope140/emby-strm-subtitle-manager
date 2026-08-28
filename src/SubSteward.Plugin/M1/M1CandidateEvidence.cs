using System;
using System.Linq;

namespace SubSteward.Plugin.M1
{
    /// <summary>
    /// Identifies metadata that strongly suggests a short-form or fan-edit
    /// source. A marker alone never proves the content is wrong; callers must
    /// combine it with the absence of a provider hash match before blocking.
    /// </summary>
    public static class M1CandidateEvidence
    {
        private static readonly string[] StrongNonFullReleaseMarkers =
        {
            "bilibili",
            "bilibilijj",
            "clip",
            "trailer",
            "teaser",
            "片段",
            "戏份",
            "花絮",
            "预告",
            "混剪",
            "弹幕"
        };

        public static bool LooksLikeNonFullRelease(string candidateName)
        {
            if (string.IsNullOrWhiteSpace(candidateName))
            {
                return false;
            }

            var normalized = candidateName.Trim().ToLowerInvariant();
            if (StrongNonFullReleaseMarkers.Any(marker => normalized.IndexOf(marker, StringComparison.Ordinal) >= 0))
            {
                return true;
            }

            return normalized.IndexOf("cut", StringComparison.Ordinal) >= 0
                && (normalized.IndexOf("戏份", StringComparison.Ordinal) >= 0
                    || normalized.IndexOf("片段", StringComparison.Ordinal) >= 0
                    || normalized.IndexOf("clip", StringComparison.Ordinal) >= 0);
        }
    }
}
