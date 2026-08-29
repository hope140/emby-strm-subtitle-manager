using System;
using System.Collections.Generic;
using System.Linq;
using System.Text;
using System.Text.RegularExpressions;
using SubSteward.Plugin.M1;
using SubSteward.Plugin.M2;

namespace SubSteward.Plugin.M3
{
    public static class M3ReferenceSyncNames
    {
        public const string Pass = "PASS";

        public const string Unknown = "UNKNOWN";

        public const string Drift = "DRIFT";
    }

    public sealed class M3ReferenceAlignmentResult
    {
        public bool IsAligned { get; set; }

        public string Status { get; set; }

        public string Method { get; set; }

        public int OffsetMilliseconds { get; set; }

        public int MatchCount { get; set; }

        public int CandidateCueCount { get; set; }

        public int ReferenceCueCount { get; set; }

        public double Coverage { get; set; }

        public double MedianAbsoluteResidualMilliseconds { get; set; }

        public double MaximumAbsoluteResidualMilliseconds { get; set; }

        public List<string> Reasons { get; } = new List<string>();
    }

    public sealed class M3CandidateAlignmentInput
    {
        public M1ValidationResult Validation { get; set; }

        public string Language { get; set; }

        public bool InstallEligible { get; set; }

        public bool TargetLanguagePresent { get; set; }

        public double PreferenceScore { get; set; }
    }

    public sealed class M3ConsensusAlignmentResult
    {
        public bool IsAligned { get; set; }

        public string Status { get; set; }

        public int SelectedCandidateIndex { get; set; } = -1;

        public M3ReferenceAlignmentResult Alignment { get; set; }

        public List<string> Reasons { get; } = new List<string>();
    }

    /// <summary>
    /// Compares fetched subtitles with one another and identifies a stable
    /// consensus timeline. It never reads video or audio.
    /// </summary>
    public sealed class M3ReferenceSubtitleAligner
    {
        public const int MinimumMatchCount = 8;

        public const int MaximumMedianResidualMilliseconds = 250;

        public const int MaximumAbsoluteOffsetMilliseconds = M1SubtitleTimelineShifter.MaxAbsoluteOffsetMilliseconds;

        public const int MinimumConsensusCandidateCount = 2;

        public const int ConsensusToleranceMilliseconds = MaximumMedianResidualMilliseconds;

        private static readonly Regex AssOverrideTag = new Regex(@"\{[^}]*\}", RegexOptions.CultureInvariant);
        private static readonly Regex HtmlTag = new Regex(@"<[^>]+>", RegexOptions.CultureInvariant);

        public M3ReferenceAlignmentResult Align(
            IEnumerable<M1Cue> candidateCues,
            IEnumerable<M1Cue> referenceCues,
            bool sameLanguage)
        {
            if (candidateCues == null)
            {
                throw new ArgumentNullException(nameof(candidateCues));
            }

            if (referenceCues == null)
            {
                throw new ArgumentNullException(nameof(referenceCues));
            }

            var candidates = PrepareCues(candidateCues);
            var references = PrepareCues(referenceCues);
            var result = new M3ReferenceAlignmentResult
            {
                Status = M3ReferenceSyncNames.Unknown,
                CandidateCueCount = candidates.Count,
                ReferenceCueCount = references.Count
            };

            if (candidates.Count == 0 || references.Count == 0)
            {
                result.Reasons.Add("参考字幕或目标字幕没有可用对白");
                return result;
            }

            List<M3CuePair> pairs;
            var sequenceComparison = false;
            if (sameLanguage)
            {
                pairs = MatchByText(candidates, references);
                result.Method = "TEXT";
                if (pairs.Count < MinimumMatchCount || pairs.Count / (double)Math.Max(1, Math.Min(candidates.Count, references.Count)) < 0.2d)
                {
                    var sequencePairs = MatchBySequence(candidates, references);
                    if (sequencePairs.Count >= MinimumMatchCount)
                    {
                        pairs = sequencePairs;
                        result.Method = "SEQUENCE";
                        sequenceComparison = true;
                    }
                }
            }
            else
            {
                pairs = MatchBySequence(candidates, references);
                result.Method = "SEQUENCE";
                sequenceComparison = true;
            }

            result.MatchCount = pairs.Count;
            result.Coverage = sequenceComparison
                ? pairs.Count / (double)Math.Max(1, Math.Min(40, Math.Min(candidates.Count, references.Count)))
                : pairs.Count / (double)Math.Max(1, Math.Min(candidates.Count, references.Count));
            if (pairs.Count < MinimumMatchCount || result.Coverage < 0.2d)
            {
                result.Reasons.Add("参考字幕与目标字幕没有足够的可匹配对白");
                return result;
            }

            var firstCandidateStart = pairs.Min(pair => pair.Candidate.StartMilliseconds);
            var lastCandidateStart = pairs.Max(pair => pair.Candidate.StartMilliseconds);
            if (lastCandidateStart - firstCandidateStart < 60000 && pairs.Count < 20)
            {
                result.Reasons.Add("匹配点没有覆盖足够长的时间范围");
                return result;
            }

            var deltas = pairs
                .Select(pair => (double)pair.Reference.StartMilliseconds - pair.Candidate.StartMilliseconds)
                .OrderBy(value => value)
                .ToList();
            var median = Median(deltas);
            if (Math.Abs(median) > MaximumAbsoluteOffsetMilliseconds)
            {
                result.Status = M3ReferenceSyncNames.Drift;
                result.Reasons.Add("参考字幕与目标字幕的固定偏移超过安全范围");
                return result;
            }

            var residuals = deltas.Select(value => Math.Abs(value - median)).OrderBy(value => value).ToList();
            result.OffsetMilliseconds = Convert.ToInt32(Math.Round(median, MidpointRounding.AwayFromZero));
            result.MedianAbsoluteResidualMilliseconds = Median(residuals);
            result.MaximumAbsoluteResidualMilliseconds = residuals[residuals.Count - 1];
            if (result.MedianAbsoluteResidualMilliseconds > MaximumMedianResidualMilliseconds)
            {
                result.Status = M3ReferenceSyncNames.Drift;
                result.Reasons.Add("参考字幕与目标字幕的固定偏移不稳定，可能存在版本差异或时间漂移");
                return result;
            }

            result.IsAligned = true;
            result.Status = M3ReferenceSyncNames.Pass;
            result.Reasons.Add("参考字幕与目标字幕的固定偏移稳定");
            return result;
        }

        public M3ConsensusAlignmentResult FindConsensus(IReadOnlyList<M3CandidateAlignmentInput> candidates)
        {
            if (candidates == null)
            {
                throw new ArgumentNullException(nameof(candidates));
            }

            var result = new M3ConsensusAlignmentResult
            {
                Status = M3ReferenceSyncNames.Unknown
            };
            var usableIndices = candidates
                .Select((candidate, index) => new { Candidate = candidate, Index = index })
                .Where(entry => IsUsableConsensusCandidate(entry.Candidate))
                .Select(entry => entry.Index)
                .ToList();
            if (usableIndices.Count < MinimumConsensusCandidateCount)
            {
                return UnknownConsensus(
                    result,
                    "至少需要两个可校验的已抓取候选字幕才能建立时间轴共识");
            }

            var edges = new List<M3CandidateAlignmentEdge>();
            var hasDrift = false;
            for (var leftIndex = 0; leftIndex < usableIndices.Count; leftIndex++)
            {
                for (var rightIndex = leftIndex + 1; rightIndex < usableIndices.Count; rightIndex++)
                {
                    var left = usableIndices[leftIndex];
                    var right = usableIndices[rightIndex];
                    var alignment = Align(
                        candidates[left].Validation.Cues,
                        candidates[right].Validation.Cues,
                        AreSameLanguage(candidates[left].Language, candidates[right].Language));
                    if (alignment.IsAligned)
                    {
                        edges.Add(new M3CandidateAlignmentEdge
                        {
                            LeftIndex = left,
                            RightIndex = right,
                            OffsetMilliseconds = alignment.OffsetMilliseconds,
                            Alignment = alignment
                        });
                    }
                    else if (string.Equals(alignment.Status, M3ReferenceSyncNames.Drift, StringComparison.Ordinal))
                    {
                        hasDrift = true;
                    }
                }
            }

            if (edges.Count == 0)
            {
                if (hasDrift)
                {
                    result.Status = M3ReferenceSyncNames.Drift;
                    result.Reasons.Add("已抓取候选字幕之间存在时间漂移，无法建立稳定共识");
                    result.Alignment = CreateFailedAlignment(M3ReferenceSyncNames.Drift, result.Reasons.Last());
                    return result;
                }

                return UnknownConsensus(result, "已抓取候选字幕没有足够的可匹配对白");
            }

            var component = FindBestComponent(usableIndices, edges, candidates);
            if (component.Count < MinimumConsensusCandidateCount)
            {
                return UnknownConsensus(
                    result,
                    "已抓取候选字幕没有形成至少两个相互可校验的时间轴");
            }

            var positions = BuildPositions(component, edges, out var hasConflict);
            if (hasConflict)
            {
                result.Status = M3ReferenceSyncNames.Drift;
                result.Reasons.Add("候选字幕的两两偏移互相冲突，可能存在版本差异或时间漂移");
                result.Alignment = CreateFailedAlignment(M3ReferenceSyncNames.Drift, result.Reasons.Last());
                return result;
            }

            var medianPosition = Median(positions.Values.OrderBy(value => value).ToList());
            var residuals = positions.Values
                .Select(value => Math.Abs(value - medianPosition))
                .OrderBy(value => value)
                .ToList();
            var consensusMembers = new HashSet<int>(positions
                .Where(entry => Math.Abs(entry.Value - medianPosition) <= ConsensusToleranceMilliseconds)
                .Select(entry => entry.Key));
            if (consensusMembers.Count < MinimumConsensusCandidateCount)
            {
                result.Status = M3ReferenceSyncNames.Drift;
                result.Reasons.Add("候选字幕之间没有形成足够稳定的时间轴共识");
                result.Alignment = CreateFailedAlignment(M3ReferenceSyncNames.Drift, result.Reasons.Last());
                return result;
            }

            var selected = consensusMembers
                .Where(index => candidates[index] != null && candidates[index].InstallEligible)
                .Select(index => new { Index = index, Candidate = candidates[index] })
                .OrderByDescending(entry => entry.Candidate.PreferenceScore)
                .ThenByDescending(entry => entry.Candidate.Validation.Cues.Count)
                .FirstOrDefault();
            if (selected == null)
            {
                return UnknownConsensus(
                    result,
                    "时间轴共识中的候选均未通过安装门禁");
            }

            var offset = medianPosition - positions[selected.Index];
            if (Math.Abs(offset) > MaximumAbsoluteOffsetMilliseconds)
            {
                result.Status = M3ReferenceSyncNames.Drift;
                result.Reasons.Add("候选共识偏移超过安全范围");
                result.Alignment = CreateFailedAlignment(M3ReferenceSyncNames.Drift, result.Reasons.Last());
                return result;
            }

            var bestEdge = edges
                .Where(edge => edge.LeftIndex == selected.Index && consensusMembers.Contains(edge.RightIndex)
                    || edge.RightIndex == selected.Index && consensusMembers.Contains(edge.LeftIndex))
                .OrderByDescending(edge => edge.Alignment.MatchCount)
                .FirstOrDefault();
            var consensusAlignment = new M3ReferenceAlignmentResult
            {
                IsAligned = true,
                Status = M3ReferenceSyncNames.Pass,
                Method = "CONSENSUS",
                OffsetMilliseconds = Convert.ToInt32(Math.Round(offset, MidpointRounding.AwayFromZero)),
                CandidateCueCount = candidates[selected.Index].Validation.Cues.Count,
                ReferenceCueCount = bestEdge == null ? 0 : GetOtherCandidateCueCount(bestEdge, selected.Index, candidates),
                MatchCount = bestEdge == null ? 0 : bestEdge.Alignment.MatchCount,
                Coverage = bestEdge == null ? 0d : bestEdge.Alignment.Coverage,
                MedianAbsoluteResidualMilliseconds = Median(residuals),
                MaximumAbsoluteResidualMilliseconds = residuals.Count == 0 ? 0d : residuals[residuals.Count - 1]
            };
            consensusAlignment.Reasons.Add("多个已抓取候选字幕形成稳定时间轴共识");
            result.IsAligned = true;
            result.Status = M3ReferenceSyncNames.Pass;
            result.SelectedCandidateIndex = selected.Index;
            result.Alignment = consensusAlignment;
            result.Reasons.AddRange(consensusAlignment.Reasons);
            return result;
        }

        private static List<M3IndexedCue> PrepareCues(IEnumerable<M1Cue> cues)
        {
            return cues
                .Where(cue => cue != null
                    && cue.StartMilliseconds >= 0
                    && cue.EndMilliseconds >= cue.StartMilliseconds
                    && BuildTextKeys(cue.Text).Count > 0)
                .OrderBy(cue => cue.StartMilliseconds)
                .ThenBy(cue => cue.EndMilliseconds)
                .Select((cue, index) => new M3IndexedCue { Cue = cue, Index = index })
                .ToList();
        }

        private static bool IsUsableConsensusCandidate(M3CandidateAlignmentInput candidate)
        {
            return candidate != null
                && candidate.Validation != null
                && string.Equals(candidate.Validation.Health, "PASS", StringComparison.OrdinalIgnoreCase)
                && candidate.TargetLanguagePresent
                && candidate.Validation.Cues.Count >= MinimumMatchCount
                && candidate.Validation.Cues.Count > 0;
        }

        private static bool AreSameLanguage(string leftLanguage, string rightLanguage)
        {
            var left = M2Language.Parse(leftLanguage, string.Empty);
            var right = M2Language.Parse(rightLanguage, string.Empty);
            if (string.IsNullOrWhiteSpace(left.Code)
                || string.IsNullOrWhiteSpace(right.Code)
                || !string.Equals(left.Code, right.Code, StringComparison.OrdinalIgnoreCase))
            {
                return false;
            }

            return string.IsNullOrWhiteSpace(left.Variant)
                || string.IsNullOrWhiteSpace(right.Variant)
                || string.Equals(left.Variant, right.Variant, StringComparison.OrdinalIgnoreCase);
        }

        private static HashSet<int> FindBestComponent(
            IReadOnlyList<int> usableIndices,
            IReadOnlyList<M3CandidateAlignmentEdge> edges,
            IReadOnlyList<M3CandidateAlignmentInput> candidates)
        {
            var remaining = new HashSet<int>(usableIndices);
            var best = new HashSet<int>();
            while (remaining.Count > 0)
            {
                var start = remaining.First();
                var component = new HashSet<int> { start };
                var queue = new Queue<int>();
                queue.Enqueue(start);
                remaining.Remove(start);
                while (queue.Count > 0)
                {
                    var current = queue.Dequeue();
                    foreach (var edge in edges.Where(edge => edge.LeftIndex == current || edge.RightIndex == current))
                    {
                        var next = edge.LeftIndex == current ? edge.RightIndex : edge.LeftIndex;
                        if (component.Add(next))
                        {
                            remaining.Remove(next);
                            queue.Enqueue(next);
                        }
                    }
                }

                if (component.Count > best.Count
                    || component.Count == best.Count && ComponentScore(component, candidates) > ComponentScore(best, candidates))
                {
                    best = component;
                }
            }

            return best;
        }

        private static double ComponentScore(HashSet<int> component, IReadOnlyList<M3CandidateAlignmentInput> candidates)
        {
            return component.Sum(index => candidates[index]?.PreferenceScore ?? 0d);
        }

        private static Dictionary<int, double> BuildPositions(
            HashSet<int> component,
            IReadOnlyList<M3CandidateAlignmentEdge> edges,
            out bool hasConflict)
        {
            hasConflict = false;
            var positions = new Dictionary<int, double>();
            var start = component.First();
            positions[start] = 0d;
            var queue = new Queue<int>();
            queue.Enqueue(start);
            while (queue.Count > 0)
            {
                var current = queue.Dequeue();
                foreach (var edge in edges.Where(edge => edge.LeftIndex == current || edge.RightIndex == current))
                {
                    var next = edge.LeftIndex == current ? edge.RightIndex : edge.LeftIndex;
                    if (!component.Contains(next))
                    {
                        continue;
                    }

                    var expected = edge.LeftIndex == current
                        ? positions[current] + edge.OffsetMilliseconds
                        : positions[current] - edge.OffsetMilliseconds;
                    if (positions.TryGetValue(next, out var existing))
                    {
                        if (Math.Abs(existing - expected) > ConsensusToleranceMilliseconds)
                        {
                            hasConflict = true;
                        }

                        continue;
                    }

                    positions[next] = expected;
                    queue.Enqueue(next);
                }
            }

            return positions;
        }

        private static int GetOtherCandidateCueCount(
            M3CandidateAlignmentEdge edge,
            int selectedIndex,
            IReadOnlyList<M3CandidateAlignmentInput> candidates)
        {
            var otherIndex = edge.LeftIndex == selectedIndex ? edge.RightIndex : edge.LeftIndex;
            return candidates[otherIndex].Validation.Cues.Count;
        }

        private static M3ConsensusAlignmentResult UnknownConsensus(M3ConsensusAlignmentResult result, string reason)
        {
            result.Status = M3ReferenceSyncNames.Unknown;
            result.Reasons.Add(reason);
            result.Alignment = CreateFailedAlignment(M3ReferenceSyncNames.Unknown, reason);
            return result;
        }

        private static M3ReferenceAlignmentResult CreateFailedAlignment(string status, string reason)
        {
            var alignment = new M3ReferenceAlignmentResult
            {
                Status = status
            };
            alignment.Reasons.Add(reason);
            return alignment;
        }

        private static List<M3CuePair> MatchByText(List<M3IndexedCue> candidates, List<M3IndexedCue> references)
        {
            var referenceByKey = new Dictionary<string, List<M3IndexedCue>>(StringComparer.Ordinal);
            foreach (var reference in references)
            {
                foreach (var key in BuildTextKeys(reference.Cue.Text))
                {
                    if (!referenceByKey.TryGetValue(key, out var entries))
                    {
                        entries = new List<M3IndexedCue>();
                        referenceByKey[key] = entries;
                    }

                    entries.Add(reference);
                }
            }

            var usedReferences = new HashSet<int>();
            var pairs = new List<M3CuePair>();
            var lastReferenceIndex = -1;
            foreach (var candidate in candidates)
            {
                M3IndexedCue selected = null;
                foreach (var key in BuildTextKeys(candidate.Cue.Text))
                {
                    if (!referenceByKey.TryGetValue(key, out var entries))
                    {
                        continue;
                    }

                    selected = entries.FirstOrDefault(entry => entry.Index > lastReferenceIndex && !usedReferences.Contains(entry.Index));
                    if (selected != null)
                    {
                        break;
                    }
                }

                if (selected == null)
                {
                    continue;
                }

                usedReferences.Add(selected.Index);
                lastReferenceIndex = selected.Index;
                pairs.Add(new M3CuePair
                {
                    Candidate = candidate.Cue,
                    Reference = selected.Cue
                });
            }

            return pairs;
        }

        private static List<M3CuePair> MatchBySequence(List<M3IndexedCue> candidates, List<M3IndexedCue> references)
        {
            if (candidates.Count < MinimumMatchCount || references.Count < MinimumMatchCount)
            {
                return new List<M3CuePair>();
            }

            var ratio = candidates.Count / (double)references.Count;
            if (ratio < 0.75d || ratio > 1.33d)
            {
                return new List<M3CuePair>();
            }

            var sampleCount = Math.Min(40, Math.Min(candidates.Count, references.Count));
            var pairs = new List<M3CuePair>();
            for (var index = 0; index < sampleCount; index++)
            {
                var candidateIndex = sampleCount == 1
                    ? 0
                    : Convert.ToInt32(Math.Round(index * (candidates.Count - 1) / (double)(sampleCount - 1), MidpointRounding.AwayFromZero));
                var referenceIndex = sampleCount == 1
                    ? 0
                    : Convert.ToInt32(Math.Round(index * (references.Count - 1) / (double)(sampleCount - 1), MidpointRounding.AwayFromZero));
                pairs.Add(new M3CuePair
                {
                    Candidate = candidates[candidateIndex].Cue,
                    Reference = references[referenceIndex].Cue
                });
            }

            return pairs;
        }

        private static List<string> BuildTextKeys(string text)
        {
            var stripped = HtmlTag.Replace(AssOverrideTag.Replace(text ?? string.Empty, string.Empty), string.Empty);
            var full = new StringBuilder(stripped.Length);
            var han = new StringBuilder(stripped.Length);
            var latin = new StringBuilder(stripped.Length);
            foreach (var character in stripped)
            {
                if (char.IsLetterOrDigit(character))
                {
                    full.Append(char.ToLowerInvariant(character));
                    latin.Append(char.ToLowerInvariant(character));
                }

                if ((character >= '\u3400' && character <= '\u4dbf')
                    || (character >= '\u4e00' && character <= '\u9fff')
                    || (character >= '\uf900' && character <= '\ufaff'))
                {
                    han.Append(character);
                }
            }

            return new[] { full.ToString(), han.ToString(), latin.ToString() }
                .Where(value => value.Length >= 2)
                .Distinct(StringComparer.Ordinal)
                .ToList();
        }

        private static double Median(IList<double> values)
        {
            if (values == null || values.Count == 0)
            {
                return 0d;
            }

            var middle = values.Count / 2;
            return values.Count % 2 == 0
                ? (values[middle - 1] + values[middle]) / 2d
                : values[middle];
        }

        private sealed class M3IndexedCue
        {
            public M1Cue Cue { get; set; }

            public int Index { get; set; }
        }

        private sealed class M3CuePair
        {
            public M1Cue Candidate { get; set; }

            public M1Cue Reference { get; set; }
        }

        private sealed class M3CandidateAlignmentEdge
        {
            public int LeftIndex { get; set; }

            public int RightIndex { get; set; }

            public int OffsetMilliseconds { get; set; }

            public M3ReferenceAlignmentResult Alignment { get; set; }
        }
    }
}
