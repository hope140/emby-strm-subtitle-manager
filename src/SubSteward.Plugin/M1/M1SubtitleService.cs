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
using SubSteward.Plugin.M2;

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

        public M2ActionReport Action { get; set; }
    }

    public sealed class M1SourceResponse
    {
        public string Id { get; set; }

        public string Name { get; set; }

        public string Container { get; set; }

        public bool IsRemote { get; set; }

        public int SubtitleStreamCount { get; set; }

        public M2PresenceReport Presence { get; set; }
    }

    public sealed class M1CandidateResponse
    {
        public string Token { get; set; }

        public string Provider { get; set; }

        public string Name { get; set; }

        public string Language { get; set; }

        public string LanguageLabel { get; set; }

        public string RequestedLanguageVariant { get; set; }

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

        public string LanguageLabel { get; set; }

        public string RequestedLanguageVariant { get; set; }

        public List<M1CandidateResponse> Candidates { get; } = new List<M1CandidateResponse>();
    }

    public sealed class M1ArtifactResponse
    {
        public string ArtifactToken { get; set; }

        public string ItemId { get; set; }

        public string MediaSourceId { get; set; }

        public string Language { get; set; }

        public string LanguageLabel { get; set; }

        public string RequestedLanguageVariant { get; set; }

        public string Format { get; set; }

        public string Encoding { get; set; }

        public string Health { get; set; }

        public bool TitleMatch { get; set; }

        public bool HashMatch { get; set; }

        public List<string> Reasons { get; } = new List<string>();

        public M2QualityReport Quality { get; set; }

        public M2PreferenceReport Preference { get; set; }

        public M2ActionReport Action { get; set; }

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
        private const int MaxCandidates = 20;
        private const int MaxPreviewCues = 200;

        private static readonly M1TokenStore Store = new M1TokenStore();
        private static readonly M1SubtitleValidator Validator = new M1SubtitleValidator();
        private static readonly M2QualityAnalyzer QualityAnalyzer = new M2QualityAnalyzer();
        private static readonly M2PreferenceAnalyzer PreferenceAnalyzer = new M2PreferenceAnalyzer();
        private static readonly M2PresenceAnalyzer PresenceAnalyzer = new M2PresenceAnalyzer();
        private static readonly M2ActionAdvisor ActionAdvisor = new M2ActionAdvisor();

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
            var requestedLanguage = M2Options.ParseTargetLanguage(request.Language);
            var language = requestedLanguage.Code;
            var candidates = (await subtitleManager.SearchSubtitles(item, language, false, false, false, Request.CancellationToken).ConfigureAwait(false) ?? Array.Empty<RemoteSubtitleInfo>())
                .OrderByDescending(candidate => candidate.IsHashMatch.GetValueOrDefault())
                .ThenByDescending(candidate => CandidateMatchesItem(candidate.Name, item))
                .ToList();
            var response = new M1SearchResponse
            {
                ItemId = item.Id.ToString("N"),
                MediaSourceId = source.Id,
                Language = language,
                LanguageLabel = requestedLanguage.Label,
                RequestedLanguageVariant = requestedLanguage.Variant
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
                    Token = Store.AddCandidate(item.Id, source.Id, language, candidate.Id, titleMatch, hashMatch, requestedLanguage.Variant),
                    Provider = candidate.ProviderName,
                    Name = candidate.Name,
                    Language = candidate.Language ?? language,
                    LanguageLabel = M2Language.Parse(candidate.Language ?? language, requestedLanguage.Code).Label,
                    RequestedLanguageVariant = requestedLanguage.Variant,
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

            var artifactToken = Store.AddArtifact(item.Id, source.Id, candidate.Language, content, validation, candidate.TitleMatch, candidate.HashMatch, candidate.RequestedLanguageVariant);
            return ToArtifactResponse(artifactToken, item.Id, source.Id, candidate.Language, validation, candidate.TitleMatch, candidate.HashMatch, candidate.RequestedLanguageVariant);
        }

        public object Get(M1PreviewSubtitleRequest request)
        {
            M1ArtifactRecord artifact;
            if (!Store.TryGetArtifact(request.ArtifactToken, out artifact))
            {
                throw new ArgumentException("Artifact token is expired or invalid.");
            }

            return ToArtifactResponse(artifact.Token, artifact.ItemId, artifact.MediaSourceId, artifact.Language, artifact.Validation, artifact.TitleMatch, artifact.HashMatch, artifact.RequestedLanguageVariant);
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
            return M2Options.ParseTargetLanguage(language).Code;
        }

        private static string GetTargetLanguage()
        {
            var configuration = Plugin.Instance?.Configuration;
            return M2Options.NormalizeTargetLanguage(configuration?.TargetLanguage);
        }

        private static string GetSecondaryLanguage()
        {
            var configuration = Plugin.Instance?.Configuration;
            return M2Options.NormalizeSecondaryLanguage(configuration?.SecondaryLanguage);
        }

        private static M2PreferenceOptions GetPreferenceOptions()
        {
            var configuration = Plugin.Instance?.Configuration;
            return M2Options.ParsePreferenceOptions(
                configuration?.PreferBilingual ?? false,
                configuration?.FormatOrder);
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
                var subtitleStreams = source.MediaStreams == null
                    ? new List<M2SubtitleStreamSnapshot>()
                    : source.MediaStreams
                        .Where(stream => stream.Type.ToString() == "Subtitle")
                        .Select(stream => new M2SubtitleStreamSnapshot
                        {
                            IsExternal = stream.IsExternal,
                            Language = stream.Language,
                            Title = string.IsNullOrWhiteSpace(stream.DisplayTitle) ? stream.Title : stream.DisplayTitle,
                            Path = stream.IsExternal ? stream.Path : null
                        })
                        .ToList();

                response.MediaSources.Add(new M1SourceResponse
                {
                    Id = source.Id,
                    Name = source.Name,
                    Container = source.Container,
                    IsRemote = source.IsRemote,
                    SubtitleStreamCount = source.MediaStreams == null ? 0 : source.MediaStreams.Count(stream => stream.Type.ToString() == "Subtitle"),
                    Presence = PresenceAnalyzer.Analyze(subtitleStreams, GetTargetLanguage(), GetSecondaryLanguage())
                });
            }

            // Presence does not include health. Keep a present target manual until
            // a health result is supplied instead of silently treating presence as PASS.
            response.Action = ActionAdvisor.Advise(new M2ActionInput
            {
                SourceCount = response.MediaSources.Count,
                TargetLanguagePresent = response.MediaSources.Count == 1 && response.MediaSources[0].Presence != null
                    ? (bool?)response.MediaSources[0].Presence.TargetLanguagePresent
                    : null
            });

            return response;
        }

        private static M1ArtifactResponse ToArtifactResponse(string token, Guid itemId, string sourceId, string language, M1ValidationResult validation, bool titleMatch, bool hashMatch, string requestedLanguageVariant)
        {
            var response = new M1ArtifactResponse
            {
                ArtifactToken = token,
                ItemId = itemId.ToString("N"),
                MediaSourceId = sourceId,
                Language = language,
                LanguageLabel = M2Options.ParseTargetLanguage(language).Label,
                RequestedLanguageVariant = requestedLanguageVariant,
                Format = validation.Format,
                Encoding = validation.Encoding,
                Health = validation.Health,
                TitleMatch = titleMatch,
                HashMatch = hashMatch
            };
            response.Reasons.AddRange(validation.Reasons);
            var secondaryLanguage = GetSecondaryLanguage();
            var preferenceOptions = GetPreferenceOptions();
            response.Quality = QualityAnalyzer.Analyze(validation, language, secondaryLanguage);
            response.Preference = PreferenceAnalyzer.Evaluate(
                validation,
                language,
                secondaryLanguage,
                token,
                titleMatch,
                hashMatch,
                preferenceOptions);
            response.Action = ActionAdvisor.Advise(new M2ActionInput
            {
                SourceCount = 1,
                TargetLanguagePresent = null,
                CandidateAvailable = true,
                CandidateHealth = response.Quality.Health,
                PreferenceSuitability = response.Preference.Suitability,
                TitleMatch = titleMatch,
                HashMatch = hashMatch,
                PreferBilingual = preferenceOptions.PreferBilingual,
                BilingualDetected = response.Quality.BilingualDetected,
                BilingualConfidence = response.Quality.BilingualConfidence
            });
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
            var requestedVariant = M2Language.Parse(artifact.RequestedLanguageVariant, artifact.Language).Variant;
            var languageTag = M2Options.ResolveSubtitleLanguageTag(requestedVariant, artifact.Language);
            var contentTypeLabel = M2SidecarLabel.Build(artifact.Validation, requestedVariant);

            var stem = baseName;
            if (!string.IsNullOrWhiteSpace(contentTypeLabel))
            {
                stem += "." + contentTypeLabel;
            }

            stem += "." + languageTag;
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
