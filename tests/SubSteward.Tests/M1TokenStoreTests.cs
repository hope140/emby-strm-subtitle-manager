using System;
using SubSteward.Plugin.M1;
using Xunit;

namespace SubSteward.Tests;

public sealed class M1TokenStoreTests
{
    [Fact]
    public void CandidateAndArtifactTokensResolveWithoutExposingRawProviderId()
    {
        var store = new M1TokenStore();
        var itemId = Guid.NewGuid();
        var candidateToken = store.AddCandidate(itemId, "source", "zho", "provider-private-id", true, false, "zh-Hans");

        Assert.NotEqual("provider-private-id", candidateToken);
        Assert.True(store.TryGetCandidate(candidateToken, out var candidate));
        Assert.Equal("provider-private-id", candidate.RawId);
        Assert.Equal("zh-Hans", candidate.RequestedLanguageVariant);

        var artifactToken = store.AddArtifact(itemId, "source", "zho", new byte[] { 1, 2 }, new M1ValidationResult { Format = "srt", Health = "PASS" }, true, false, candidate.RequestedLanguageVariant, 500);
        Assert.True(store.TryGetArtifact(artifactToken, out var artifact));
        Assert.Equal(itemId, artifact.ItemId);
        Assert.Equal("zh-Hans", artifact.RequestedLanguageVariant);
        Assert.Equal(500, artifact.TimelineOffsetMilliseconds);
        Assert.True(store.RemoveArtifact(artifactToken));
        Assert.False(store.TryGetArtifact(artifactToken, out _));
    }
}
