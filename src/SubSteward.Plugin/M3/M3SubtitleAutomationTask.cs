using System;
using System.Collections.Generic;
using System.Threading;
using System.Threading.Tasks;
using MediaBrowser.Controller.Library;
using MediaBrowser.Controller.Providers;
using MediaBrowser.Controller.Subtitles;
using MediaBrowser.Model.IO;
using MediaBrowser.Model.Logging;
using MediaBrowser.Model.Tasks;

namespace SubSteward.Plugin.M3
{
    /// <summary>
    /// Emby scheduled task for M3 automatic missing-subtitle supplementation.
    /// The task is always visible to the host, but its default configuration
    /// is disabled and dry-run, so registration alone cannot scan or write.
    /// </summary>
    public sealed class M3SubtitleAutomationTask : IScheduledTask
    {
        public const string TaskKey = "SubStewardAutomaticSubtitleSupplement";

        public static readonly TimeSpan DefaultInterval = TimeSpan.FromHours(24);

        private readonly ILibraryManager libraryManager;
        private readonly ISubtitleManager subtitleManager;
        private readonly IProviderManager providerManager;
        private readonly IFileSystem fileSystem;
        private readonly ILogManager logManager;

        public M3SubtitleAutomationTask(
            ILibraryManager libraryManager,
            ISubtitleManager subtitleManager,
            IProviderManager providerManager,
            IFileSystem fileSystem,
            ILogManager logManager = null)
        {
            this.libraryManager = libraryManager ?? throw new ArgumentNullException(nameof(libraryManager));
            this.subtitleManager = subtitleManager ?? throw new ArgumentNullException(nameof(subtitleManager));
            this.providerManager = providerManager ?? throw new ArgumentNullException(nameof(providerManager));
            this.fileSystem = fileSystem ?? throw new ArgumentNullException(nameof(fileSystem));
            this.logManager = logManager;
        }

        public string Name => "SubSteward 自动补缺";

        public string Key => TaskKey;

        public string Description => "按明确授权的媒体库自动补充缺失的目标语言字幕；默认关闭并先以 dry-run 校验。";

        public string Category => "SubSteward";

        public IEnumerable<TaskTriggerInfo> GetDefaultTriggers()
        {
            return new[]
            {
                new TaskTriggerInfo
                {
                    Type = TaskTriggerInfo.TriggerInterval,
                    IntervalTicks = DefaultInterval.Ticks
                }
            };
        }

        public async Task Execute(CancellationToken cancellationToken, IProgress<double> progress)
        {
            progress?.Report(0);
            Plugin plugin;
            try
            {
                plugin = Plugin.Instance;
            }
            catch (InvalidOperationException)
            {
                progress?.Report(1);
                return;
            }

            var runner = new M3SubtitleAutomationRunner(
                libraryManager,
                subtitleManager,
                providerManager,
                fileSystem,
                logManager);
            await runner.RunExclusiveAsync(plugin.Configuration, cancellationToken, progress).ConfigureAwait(false);
            progress?.Report(1);
        }
    }
}
