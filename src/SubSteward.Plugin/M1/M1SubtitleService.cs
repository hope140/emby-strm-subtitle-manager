using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Threading;
using System.Threading.Tasks;
using MediaBrowser.Controller.Api;
using MediaBrowser.Controller.Entities;
using MediaBrowser.Controller.Library;
using MediaBrowser.Controller.Net;
using MediaBrowser.Controller.Providers;
using MediaBrowser.Controller.Subtitles;
using MediaBrowser.Model.Configuration;
using MediaBrowser.Model.Dto;
using MediaBrowser.Model.IO;
using MediaBrowser.Model.Providers;
using MediaBrowser.Model.Services;

namespace SubSteward.Plugin.M1
{
    [Route("/SubSteward/Items", "GET", Summary = "Lists SubSteward-managed media items")]
    [Authenticated(Roles = "Admin")]
    public sealed class M1ListItemsRequest
    {
        public string SearchTerm { get; set; }

        public int? Limit { get; set; }
    }

    [Route("/SubSteward/Items/{Id}", "GET", Summary = "Gets a SubSteward media item")]
    [Authenticated(Roles = "Admin")]
    public sealed class M1GetItemRequest
    {
        public string Id { get; set; }
    }

    [Route("/SubSteward/Subtitles/Search", "GET", Summary = "Searches subtitle candidates")]
    [Authenticated(Roles = "Admin")]
    public sealed class M1SearchSubtitlesRequest
    {
        public string ItemId { get; set; }

        public string MediaSourceId { get; set; }

        public string Language { get; set; }
    }

    [Route("/SubSteward/Subtitles/Fetch", "POST", Summary = "Fetches and validates one subtitle candidate")]
    [Authenticated(Roles = "Admin")]
    public sealed class M1FetchSubtitleRequest
    {
        public string CandidateToken { get; set; }
    }

    [Route("/SubSteward/Subtitles/Preview", "GET", Summary = "Previews one fetched subtitle artifact")]
    [Authenticated(Roles = "Admin")]
    public sealed class M1PreviewSubtitleRequest
    {
        public string ArtifactToken { get; set; }
    }

    [Route("/SubSteward/Subtitles/Install", "POST", Summary = "Installs one validated subtitle and refreshes the item")]
    [Authenticated(Roles = "Admin")]
    public sealed class M1InstallSubtitleRequest
    {
        public string ArtifactToken { get; set; }
    }

    public sealed class M1ItemResponse
    {
        public string Id { get; set; }

        public string Name { get; set; }

        public string Type { get; set; }

        public bool IsStrm { get; set; }

        public List<M1SourceResponse> MediaSources { get; } = new List<M1SourceResponse>();
    }

    public sealed class M1SourceResponse
    {
        public string Id { get; set; }

        public string Name { get; set; }

        public string Container { get; set; }

        public bool IsRemote { get; set; }

        public int SubtitleStreamCount { get; set; }
    }

    public sealed class M1CandidateResponse
    {
        public string Token { get; set; }

        public string Provider { get; set; }

        public string Name { get; set; }

        public string Language { get; set; }

        public string Format { get; set; }

        public string Author { get; set; }

        public bool? IsHashMatch { get; set; }

        public bool TitleMatch { get; set; }
    }

    public sealed class M1SearchResponse
    {
        public string ItemId { get; set; }

        public string MediaSourceId { get; set; }

        public string Language { get; set; }

        public List<M1CandidateResponse> Candidates { get; } = new List<M1CandidateResponse>();
    }

    public sealed class M1ArtifactResponse
    {
        public string ArtifactToken { get; set; }

        public string ItemId { get; set; }

        public string MediaSourceId { get; set; }

        public string Language { get; set; }

        public string Format { get; set; }

        public string Encoding { get; set; }

        public string Health { get; set; }

        public bool TitleMatch { get; set; }

        public bool HashMatch { get; set; }

        public List<string> Reasons { get; } = new List<string>();

        public List<M1Cue> Cues { get; } = new List<M1Cue>();
    }

    public sealed class M1InstallResponse
    {
        public string ItemId { get; set; }

        public string MediaSourceId { get; set; }

        public string Language { get; set; }

        public string Format { get; set; }

        public string FileName { get; set; }

        public int ExternalSubtitleStreamCount { get; set; }
    }

    public sealed class M1SubtitleService : BaseApiService
    {
        private const string DefaultLanguage = "zho";
        private const int MaxCandidates = 20;
        private const int MaxPreviewCues = 200;

        private static readonly M1TokenStore Store = new M1TokenStore();
        private static readonly M1SubtitleValidator Validator = new M1SubtitleValidator();

        private readonly ILibraryManager libraryManager;
        private readonly ISubtitleManager subtitleManager;
        private readonly IProviderManager providerManager;
        private readonly IFileSystem fileSystem;

        public M1SubtitleService(
            ILibraryManager libraryManager,
            ISubtitleManager subtitleManager,
            IProviderManager providerManager,
            IFileSystem fileSystem)
        {
            this.libraryManager = libraryManager;
            this.subtitleManager = subtitleManager;
            this.providerManager = providerManager;
            this.fileSystem = fileSystem;
        }

        public object Get(M1ListItemsRequest request)
        {
            var limit = request.Limit.GetValueOrDefault(50);
            if (limit < 1 || limit > 100)
            {
                throw new ArgumentException("Limit must be between 1 and 100.");
            }

            var items = libraryManager.GetItemList(new InternalItemsQuery
            {
                SearchTerm = request.SearchTerm ?? string.Empty,
                IncludeItemTypes = new[] { "Movie", "Episode" },
                Recursive = true,
                Limit = limit
            });

            return items.Select(ToItemResponse).ToList();
        }

        public object Get(M1GetItemRequest request)
        {
            return ToItemResponse(ResolveItem(request.Id));
        }

        public async Task<object> Get(M1SearchSubtitlesRequest request)
        {
            var item = ResolveItem(request.ItemId);
            var source = ResolveSource(item, request.MediaSourceId);
            var language = NormalizeLanguage(request.Language);
            var candidates = (await subtitleManager.SearchSubtitles(item, language, false, false, false, Request.CancellationToken).ConfigureAwait(false) ?? Array.Empty<RemoteSubtitleInfo>())
                .OrderByDescending(candidate => candidate.IsHashMatch.GetValueOrDefault())
                .ThenByDescending(candidate => CandidateMatchesItem(candidate.Name, item))
                .ToList();
            var response = new M1SearchResponse
            {
                ItemId = item.Id.ToString("N"),
                MediaSourceId = source.Id,
                Language = language
            };

            foreach (var candidate in candidates)
            {
                if (string.IsNullOrWhiteSpace(candidate.Id))
                {
                    continue;
                }

                var titleMatch = CandidateMatchesItem(candidate.Name, item);
                var hashMatch = candidate.IsHashMatch.GetValueOrDefault();
                response.Candidates.Add(new M1CandidateResponse
                {
                    Token = Store.AddCandidate(item.Id, source.Id, language, candidate.Id, titleMatch, hashMatch),
                    Provider = candidate.ProviderName,
                    Name = candidate.Name,
                    Language = candidate.Language ?? language,
                    Format = candidate.Format,
                    Author = candidate.Author,
                    IsHashMatch = candidate.IsHashMatch,
                    TitleMatch = titleMatch
                });

                if (response.Candidates.Count >= MaxCandidates)
                {
                    break;
                }
            }

            return response;
        }

        public async Task<object> Post(M1FetchSubtitleRequest request)
        {
            M1CandidateRecord candidate;
            if (!Store.TryGetCandidate(request.CandidateToken, out candidate))
            {
                throw new ArgumentException("Candidate token is expired or invalid.");
            }

            var item = ResolveItem(candidate.ItemId.ToString("N"));
            var source = ResolveSource(item, candidate.MediaSourceId);
            if (!candidate.TitleMatch && !candidate.HashMatch)
            {
                throw new InvalidOperationException("Candidate metadata does not match the selected Item.");
            }

            var fetched = await subtitleManager.GetRemoteSubtitles(candidate.RawId, Request.CancellationToken).ConfigureAwait(false);
            if (fetched == null || fetched.Stream == null)
            {
                throw new InvalidOperationException("Emby returned no subtitle content.");
            }

            byte[] content;
            using (fetched.Stream)
            {
                content = await ReadBoundedAsync(fetched.Stream, Request.CancellationToken).ConfigureAwait(false);
            }
            var validation = Validator.Validate(content, fetched.Format);
            if (!validation.IsUsable)
            {
                throw new InvalidOperationException("Fetched subtitle failed validation.");
            }

            if (IsChineseLanguage(candidate.Language) && !validation.HasHanCharacters)
            {
                throw new InvalidOperationException("Fetched subtitle metadata says Chinese, but its content contains no Chinese characters.");
            }

            var artifactToken = Store.AddArtifact(item.Id, source.Id, candidate.Language, content, validation, candidate.TitleMatch, candidate.HashMatch);
            return ToArtifactResponse(artifactToken, item.Id, source.Id, candidate.Language, validation, candidate.TitleMatch, candidate.HashMatch);
        }

        public object Get(M1PreviewSubtitleRequest request)
        {
            M1ArtifactRecord artifact;
            if (!Store.TryGetArtifact(request.ArtifactToken, out artifact))
            {
                throw new ArgumentException("Artifact token is expired or invalid.");
            }

            return ToArtifactResponse(artifact.Token, artifact.ItemId, artifact.MediaSourceId, artifact.Language, artifact.Validation, artifact.TitleMatch, artifact.HashMatch);
        }

        public async Task<object> Post(M1InstallSubtitleRequest request)
        {
            M1ArtifactRecord artifact;
            if (!Store.TryGetArtifact(request.ArtifactToken, out artifact))
            {
                throw new ArgumentException("Artifact token is expired or invalid.");
            }

            var item = ResolveItem(artifact.ItemId.ToString("N"));
            var source = ResolveSource(item, artifact.MediaSourceId);
            var before = CountExternalSubtitleStreams(item);
            string targetPath = null;
            try
            {
                targetPath = WriteSidecar(item, source, artifact);
                await providerManager.RefreshFullItem(item, new MetadataRefreshOptions(new DirectoryService(fileSystem)), Request.CancellationToken).ConfigureAwait(false);
                var refreshed = libraryManager.GetItemById(item.Id);
                var after = refreshed == null ? before : CountExternalSubtitleStreams(refreshed);
                if (after <= before || !HasExternalSubtitleStream(refreshed, source.Id, Path.GetFileName(targetPath)))
                {
                    throw new InvalidOperationException("Emby did not report the newly installed subtitle stream after refresh.");
                }

                Store.RemoveArtifact(artifact.Token);
                return new M1InstallResponse
                {
                    ItemId = item.Id.ToString("N"),
                    MediaSourceId = source.Id,
                    Language = artifact.Language,
                    Format = artifact.Validation.Format,
                    FileName = Path.GetFileName(targetPath),
                    ExternalSubtitleStreamCount = after
                };
            }
            catch
            {
                if (!string.IsNullOrWhiteSpace(targetPath))
                {
                    TryDeleteCreatedFile(targetPath);
                }

                throw;
            }
        }

        private BaseItem ResolveItem(string itemId)
        {
            Guid parsed;
            if (!Guid.TryParse(itemId, out parsed))
            {
                throw new ArgumentException("ItemId is invalid.");
            }

            var item = libraryManager.GetItemById(parsed);
            if (item == null || (!string.Equals(item.GetType().Name, "Movie", StringComparison.Ordinal) && !string.Equals(item.GetType().Name, "Episode", StringComparison.Ordinal)))
            {
                throw new ArgumentException("Item is not a supported Movie or Episode.");
            }

            return item;
        }

        private static MediaSourceInfo ResolveSource(BaseItem item, string mediaSourceId)
        {
            var sources = item.GetMediaSources(false, false, new LibraryOptions()).ToList();
            if (sources.Count != 1)
            {
                throw new InvalidOperationException("M1 currently requires exactly one MediaSource.");
            }

            var source = sources[0];
            if (!string.IsNullOrWhiteSpace(mediaSourceId) && !string.Equals(source.Id, mediaSourceId, StringComparison.Ordinal))
            {
                throw new ArgumentException("MediaSourceId does not belong to the selected Item.");
            }

            return source;
        }

        private static string NormalizeLanguage(string language)
        {
            return string.IsNullOrWhiteSpace(language) ? DefaultLanguage : language.Trim();
        }

        private static bool IsChineseLanguage(string language)
        {
            return string.Equals(language, "zho", StringComparison.OrdinalIgnoreCase)
                || string.Equals(language, "zh", StringComparison.OrdinalIgnoreCase)
                || string.Equals(language, "zh-CN", StringComparison.OrdinalIgnoreCase)
                || string.Equals(language, "chi", StringComparison.OrdinalIgnoreCase);
        }

        private static bool CandidateMatchesItem(string candidateName, BaseItem item)
        {
            if (string.IsNullOrWhiteSpace(candidateName))
            {
                return false;
            }

            return (!string.IsNullOrWhiteSpace(item.Name) && candidateName.IndexOf(item.Name, StringComparison.OrdinalIgnoreCase) >= 0)
                || (!string.IsNullOrWhiteSpace(item.OriginalTitle) && candidateName.IndexOf(item.OriginalTitle, StringComparison.OrdinalIgnoreCase) >= 0);
        }

        private static M1ItemResponse ToItemResponse(BaseItem item)
        {
            var response = new M1ItemResponse
            {
                Id = item.Id.ToString("N"),
                Name = item.Name,
                Type = item.GetType().Name,
                IsStrm = !string.IsNullOrWhiteSpace(item.Path) && item.Path.EndsWith(".strm", StringComparison.OrdinalIgnoreCase)
            };

            foreach (var source in item.GetMediaSources(false, false, new LibraryOptions()))
            {
                response.MediaSources.Add(new M1SourceResponse
                {
                    Id = source.Id,
                    Name = source.Name,
                    Container = source.Container,
                    IsRemote = source.IsRemote,
                    SubtitleStreamCount = source.MediaStreams == null ? 0 : source.MediaStreams.Count(stream => stream.Type.ToString() == "Subtitle")
                });
            }

            return response;
        }

        private static M1ArtifactResponse ToArtifactResponse(string token, Guid itemId, string sourceId, string language, M1ValidationResult validation, bool titleMatch, bool hashMatch)
        {
            var response = new M1ArtifactResponse
            {
                ArtifactToken = token,
                ItemId = itemId.ToString("N"),
                MediaSourceId = sourceId,
                Language = language,
                Format = validation.Format,
                Encoding = validation.Encoding,
                Health = validation.Health,
                TitleMatch = titleMatch,
                HashMatch = hashMatch
            };
            response.Reasons.AddRange(validation.Reasons);
            response.Cues.AddRange(validation.Cues.Take(MaxPreviewCues));
            return response;
        }

        private static int CountExternalSubtitleStreams(BaseItem item)
        {
            return item.GetMediaSources(false, false, new LibraryOptions())
                .Sum(source => source.MediaStreams == null ? 0 : source.MediaStreams.Count(stream => stream.IsExternal && stream.Type.ToString() == "Subtitle"));
        }

        private static async Task<byte[]> ReadBoundedAsync(Stream stream, CancellationToken cancellationToken)
        {
            const int maxBytes = 16 * 1024 * 1024;
            using (var output = new MemoryStream())
            {
                var buffer = new byte[81920];
                while (true)
                {
                    var read = await stream.ReadAsync(buffer, 0, buffer.Length, cancellationToken).ConfigureAwait(false);
                    if (read == 0)
                    {
                        break;
                    }

                    if (output.Length + read > maxBytes)
                    {
                        throw new InvalidOperationException("Fetched subtitle exceeds the M1 size limit.");
                    }

                    output.Write(buffer, 0, read);
                }

                return output.ToArray();
            }
        }

        private static string WriteSidecar(BaseItem item, MediaSourceInfo source, M1ArtifactRecord artifact)
        {
            var anchor = item.Path;
            if (item.Path.EndsWith(".strm", StringComparison.OrdinalIgnoreCase))
            {
                EnsureRegularFile(anchor, "STRM Item.Path");
            }
            else
            {
                if (source.IsRemote || string.IsNullOrWhiteSpace(source.Path))
                {
                    throw new InvalidOperationException("The selected non-STRM source is not a local filesystem path.");
                }

                anchor = source.Path;
                EnsureRegularFile(anchor, "MediaSource.Path");
            }

            var directory = Path.GetDirectoryName(Path.GetFullPath(anchor));
            var baseName = Path.GetFileNameWithoutExtension(anchor);
            if (string.IsNullOrWhiteSpace(directory) || string.IsNullOrWhiteSpace(baseName))
            {
                throw new InvalidOperationException("The selected media path cannot produce a sidecar target.");
            }

            var directoryAttributes = File.GetAttributes(directory);
            if ((directoryAttributes & FileAttributes.Directory) == 0 || (directoryAttributes & FileAttributes.ReparsePoint) != 0)
            {
                throw new InvalidOperationException("The selected media directory is not a regular directory.");
            }

            var extension = artifact.Validation.Format == "ass" || artifact.Validation.Format == "ssa" ? artifact.Validation.Format : "srt";
            var stem = baseName + "." + artifact.Language;
            for (var version = 0; version < 100; version++)
            {
                var suffix = version == 0 ? string.Empty : ".v" + version;
                var target = Path.Combine(directory, stem + suffix + "." + extension);
                if (File.Exists(target))
                {
                    continue;
                }

                var temporary = Path.Combine(directory, "." + Path.GetFileName(target) + ".substeward-" + Guid.NewGuid().ToString("N") + ".tmp");
                try
                {
                    using (var output = new FileStream(temporary, FileMode.CreateNew, FileAccess.Write, FileShare.None))
                    {
                        output.Write(artifact.Content, 0, artifact.Content.Length);
                        output.Flush(true);
                    }

                    File.Move(temporary, target);
                    return target;
                }
                finally
                {
                    TryDeleteCreatedFile(temporary);
                }
            }

            throw new IOException("Unable to allocate a new subtitle sidecar filename.");
        }

        private static void EnsureRegularFile(string path, string label)
        {
            if (string.IsNullOrWhiteSpace(path) || !File.Exists(path))
            {
                throw new InvalidOperationException(label + " does not point to an existing file.");
            }

            var attributes = File.GetAttributes(path);
            if ((attributes & FileAttributes.Directory) != 0 || (attributes & FileAttributes.ReparsePoint) != 0)
            {
                throw new InvalidOperationException(label + " is not a regular file.");
            }
        }

        private static bool HasExternalSubtitleStream(BaseItem item, string sourceId, string fileName)
        {
            return item.GetMediaSources(false, false, new LibraryOptions())
                .Where(source => string.Equals(source.Id, sourceId, StringComparison.Ordinal))
                .SelectMany(source => source.MediaStreams ?? new List<MediaBrowser.Model.Entities.MediaStream>())
                .Any(stream => stream.IsExternal && stream.Type.ToString() == "Subtitle" && !string.IsNullOrWhiteSpace(stream.Path) && stream.Path.EndsWith(fileName, StringComparison.OrdinalIgnoreCase));
        }

        private static void TryDeleteCreatedFile(string path)
        {
            try
            {
                if (!string.IsNullOrWhiteSpace(path) && File.Exists(path))
                {
                    File.Delete(path);
                }
            }
            catch (IOException)
            {
            }
            catch (UnauthorizedAccessException)
            {
            }
        }
    }
}
