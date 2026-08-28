using SubSteward.Plugin.M1;
using Xunit;

namespace SubSteward.Tests;

public sealed class M1CandidateEvidenceTests
{
    [Theory]
    [InlineData("bilibilijj.com-高清源-小时代3-顾准戏份cut.ass")]
    [InlineData("小时代3 预告片.srt")]
    [InlineData("movie clip.srt")]
    public void ShortFormMarkers_AreFlagged(string candidateName)
    {
        Assert.True(M1CandidateEvidence.LooksLikeNonFullRelease(candidateName));
    }

    [Theory]
    [InlineData("小时代3.2014.1080p.WEB-DL.srt")]
    [InlineData("Spirited Away 2001 BluRay.ass")]
    public void FullReleaseNames_AreNotFlagged(string candidateName)
    {
        Assert.False(M1CandidateEvidence.LooksLikeNonFullRelease(candidateName));
    }
}
