using System.Collections.Generic;
using SubSteward.Plugin.M1;
using SubSteward.Plugin.M3;
using Xunit;

namespace SubSteward.Tests;

public sealed class M3ReferenceSubtitleAlignerTests
{
    [Fact]
    public void SameLanguageReference_ComputesStableUniformOffset()
    {
        var reference = CreateCues(0);
        var candidate = CreateCues(2100);

        var result = new M3ReferenceSubtitleAligner().Align(candidate, reference, true);

        Assert.True(result.IsAligned);
        Assert.Equal(M3ReferenceSyncNames.Pass, result.Status);
        Assert.Equal(-2100, result.OffsetMilliseconds);
        Assert.Equal("TEXT", result.Method);
        Assert.Equal(12, result.MatchCount);
    }

    [Fact]
    public void DifferentLanguageReference_UsesCueSequenceWhenCountsAreClose()
    {
        var reference = CreateCues(0, "English line ");
        var candidate = CreateCues(1500, "中文台词 ");

        var result = new M3ReferenceSubtitleAligner().Align(candidate, reference, false);

        Assert.True(result.IsAligned);
        Assert.Equal(M3ReferenceSyncNames.Pass, result.Status);
        Assert.Equal(-1500, result.OffsetMilliseconds);
        Assert.Equal("SEQUENCE", result.Method);
    }

    [Fact]
    public void SameLanguageWithDifferentSegmentation_FallsBackToSequence()
    {
        var reference = CreateCues(0, "参考字幕 ");
        var candidate = CreateCues(1500, "另一版字幕 ");

        var result = new M3ReferenceSubtitleAligner().Align(candidate, reference, true);

        Assert.True(result.IsAligned);
        Assert.Equal(M3ReferenceSyncNames.Pass, result.Status);
        Assert.Equal("SEQUENCE", result.Method);
        Assert.Equal(-1500, result.OffsetMilliseconds);
    }

    [Fact]
    public void UnstableOffsets_AreMarkedAsDrift()
    {
        var reference = CreateCues(0);
        var candidate = CreateCuesWithGrowingOffset();

        var result = new M3ReferenceSubtitleAligner().Align(candidate, reference, true);

        Assert.False(result.IsAligned);
        Assert.Equal(M3ReferenceSyncNames.Drift, result.Status);
    }

    [Fact]
    public void TooFewMatches_RemainUnknown()
    {
        var reference = CreateCues(0);
        var candidate = new List<M1Cue>
        {
            new M1Cue { StartMilliseconds = 1000, EndMilliseconds = 2000, Text = "只匹配" }
        };

        var result = new M3ReferenceSubtitleAligner().Align(candidate, reference, true);

        Assert.False(result.IsAligned);
        Assert.Equal(M3ReferenceSyncNames.Unknown, result.Status);
    }

    [Fact]
    public void ConsensusSelectsTheMajorityTimeline()
    {
        var candidates = new[]
        {
            CreateCandidate(0, true, 90),
            CreateCandidate(0, true, 80),
            CreateCandidate(2100, true, 100)
        };

        var result = new M3ReferenceSubtitleAligner().FindConsensus(candidates);

        Assert.True(result.IsAligned);
        Assert.Equal(M3ReferenceSyncNames.Pass, result.Status);
        Assert.Equal(0, result.SelectedCandidateIndex);
        Assert.Equal("CONSENSUS", result.Alignment.Method);
        Assert.Equal(0, result.Alignment.OffsetMilliseconds);
    }

    [Fact]
    public void ConsensusRejectsTwoCandidatesWithDifferentTimelines()
    {
        var candidates = new[]
        {
            CreateCandidate(0, true, 90),
            CreateCandidate(2100, true, 80)
        };

        var result = new M3ReferenceSubtitleAligner().FindConsensus(candidates);

        Assert.False(result.IsAligned);
        Assert.Equal(M3ReferenceSyncNames.Drift, result.Status);
    }

    [Fact]
    public void ConsensusMayUseANonRecommendedCandidateAsReference()
    {
        var candidates = new[]
        {
            CreateCandidate(0, true, 90),
            CreateCandidate(0, false, 78)
        };

        var result = new M3ReferenceSubtitleAligner().FindConsensus(candidates);

        Assert.True(result.IsAligned);
        Assert.Equal(0, result.SelectedCandidateIndex);
    }

    private static List<M1Cue> CreateCues(int offset, string prefix = "台词 ")
    {
        var cues = new List<M1Cue>();
        for (var index = 0; index < 12; index++)
        {
            var start = 5000 + index * 10000 + offset;
            cues.Add(new M1Cue
            {
                Index = index + 1,
                StartMilliseconds = start,
                EndMilliseconds = start + 4000,
                Text = prefix + (index + 1)
            });
        }

        return cues;
    }

    private static List<M1Cue> CreateCuesWithGrowingOffset()
    {
        var cues = new List<M1Cue>();
        for (var index = 0; index < 12; index++)
        {
            var start = 5000 + index * 10000 + index * 500;
            cues.Add(new M1Cue
            {
                Index = index + 1,
                StartMilliseconds = start,
                EndMilliseconds = start + 4000,
                Text = "台词 " + (index + 1)
            });
        }

        return cues;
    }

    private static M3CandidateAlignmentInput CreateCandidate(int offset, bool installEligible, double preferenceScore)
    {
        return new M3CandidateAlignmentInput
        {
            Validation = new M1ValidationResult
            {
                Health = "PASS",
                Format = "srt"
            },
            Language = "zho",
            InstallEligible = installEligible,
            TargetLanguagePresent = true,
            PreferenceScore = preferenceScore
        }.WithCues(CreateCues(offset));
    }
}

internal static class M3ReferenceSubtitleAlignerTestExtensions
{
    public static M3CandidateAlignmentInput WithCues(this M3CandidateAlignmentInput candidate, List<M1Cue> cues)
    {
        candidate.Validation.Cues.AddRange(cues);
        return candidate;
    }
}
