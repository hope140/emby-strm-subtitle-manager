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
        var candidateToken = store.AddCandidate(itemId, "source", "zho", "provider-private-id", true, false);

        Assert.NotEqual("provider-private-id", candidateToken);
        Assert.True(store.TryGetCandidate(candidateToken, out var candidate));
        Assert.Equal("provider-private-id", candidate.RawId);

        var artifactToken = store.AddArtifact(itemId, "source", "zho", new byte[] { 1, 2 }, new M1ValidationResult { Format = "srt", Health = "PASS" }, true, false);
        Assert.True(store.TryGetArtifact(artifactToken, out var artifact));
        Assert.Equal(itemId, artifact.ItemId);
        Assert.True(store.RemoveArtifact(artifactToken));
        Assert.False(store.TryGetArtifact(artifactToken, out _));
    }
}
