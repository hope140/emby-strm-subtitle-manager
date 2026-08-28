using System;
using System.Collections.Generic;
using System.IO;
using System.Security;

namespace SubSteward.Plugin.M1
{
    /// <summary>
    /// Describes a bounded inspection of an existing external subtitle file.
    /// A file can be readable but invalid, so IsInspectable is kept separate
    /// from the validator Health value.
    /// </summary>
    public sealed class M1SubtitleFileInspection
    {
        public bool IsInspectable { get; set; }

        public M1ValidationResult Validation { get; set; }

        public List<string> Reasons { get; } = new List<string>();
    }

    /// <summary>
    /// Reads only a regular subtitle file beside the selected local media
    /// anchor. It never follows a remote MediaSource path or an arbitrary
    /// path outside that anchor directory.
    /// </summary>
    public sealed class M1SubtitleFileInspector
    {
        private const int MaxBytes = 16 * 1024 * 1024;

        private readonly M1SubtitleValidator validator;

        public M1SubtitleFileInspector(M1SubtitleValidator validator = null)
        {
            this.validator = validator ?? new M1SubtitleValidator();
        }

        public M1SubtitleFileInspection Inspect(string anchorPath, string subtitlePath, string format)
        {
            var result = new M1SubtitleFileInspection();
            string anchorDirectory;
            string resolvedSubtitlePath;
            if (!TryResolveSiblingFile(anchorPath, subtitlePath, out anchorDirectory, out resolvedSubtitlePath))
            {
                result.Reasons.Add("subtitle path is not a safe local sidecar beside the selected media");
                return result;
            }

            try
            {
                var attributes = File.GetAttributes(resolvedSubtitlePath);
                if ((attributes & FileAttributes.Directory) != 0 || (attributes & FileAttributes.ReparsePoint) != 0)
                {
                    result.Reasons.Add("subtitle path is not a regular file");
                    return result;
                }

                var fileInfo = new FileInfo(resolvedSubtitlePath);
                if (fileInfo.Length > MaxBytes)
                {
                    result.Reasons.Add("subtitle file exceeds the inspection size limit");
                    return result;
                }

                var content = ReadBounded(fileInfo);
                result.Validation = validator.Validate(content, format ?? Path.GetExtension(resolvedSubtitlePath));
                result.IsInspectable = true;
                result.Reasons.AddRange(result.Validation.Reasons);
                return result;
            }
            catch (FileNotFoundException)
            {
                result.Reasons.Add("subtitle file is no longer available");
            }
            catch (DirectoryNotFoundException)
            {
                result.Reasons.Add("subtitle directory is no longer available");
            }
            catch (UnauthorizedAccessException)
            {
                result.Reasons.Add("subtitle file cannot be read by the Emby process");
            }
            catch (IOException)
            {
                result.Reasons.Add("subtitle file could not be read");
            }
            catch (ArgumentException)
            {
                result.Reasons.Add("subtitle path is invalid");
            }
            catch (NotSupportedException)
            {
                result.Reasons.Add("subtitle path is not supported on this filesystem");
            }
            catch (SecurityException)
            {
                result.Reasons.Add("subtitle file access was denied");
            }

            return result;
        }

        private static bool TryResolveSiblingFile(
            string anchorPath,
            string subtitlePath,
            out string anchorDirectory,
            out string resolvedSubtitlePath)
        {
            anchorDirectory = null;
            resolvedSubtitlePath = null;
            if (string.IsNullOrWhiteSpace(anchorPath) || string.IsNullOrWhiteSpace(subtitlePath)
                || !Path.IsPathRooted(anchorPath) || !Path.IsPathRooted(subtitlePath))
            {
                return false;
            }

            try
            {
                var resolvedAnchorPath = Path.GetFullPath(anchorPath);
                resolvedSubtitlePath = Path.GetFullPath(subtitlePath);
                anchorDirectory = Path.GetDirectoryName(resolvedAnchorPath);
                var subtitleDirectory = Path.GetDirectoryName(resolvedSubtitlePath);
                if (string.IsNullOrWhiteSpace(anchorDirectory)
                    || string.IsNullOrWhiteSpace(subtitleDirectory)
                    || !string.Equals(anchorDirectory, subtitleDirectory, GetPathComparison()))
                {
                    return false;
                }

                var anchorAttributes = File.GetAttributes(resolvedAnchorPath);
                var directoryAttributes = File.GetAttributes(anchorDirectory);
                if ((anchorAttributes & (FileAttributes.Directory | FileAttributes.ReparsePoint)) != 0
                    || (directoryAttributes & (FileAttributes.Directory | FileAttributes.ReparsePoint)) != FileAttributes.Directory)
                {
                    return false;
                }

                return true;
            }
            catch (ArgumentException)
            {
                return false;
            }
            catch (IOException)
            {
                return false;
            }
            catch (NotSupportedException)
            {
                return false;
            }
            catch (UnauthorizedAccessException)
            {
                return false;
            }
            catch (SecurityException)
            {
                return false;
            }
        }

        private static StringComparison GetPathComparison()
        {
            return Path.DirectorySeparatorChar == '\\'
                ? StringComparison.OrdinalIgnoreCase
                : StringComparison.Ordinal;
        }

        private static byte[] ReadBounded(FileInfo fileInfo)
        {
            using (var input = new FileStream(fileInfo.FullName, FileMode.Open, FileAccess.Read, FileShare.Read, 81920, FileOptions.SequentialScan))
            using (var output = new MemoryStream())
            {
                var buffer = new byte[81920];
                while (true)
                {
                    var read = input.Read(buffer, 0, buffer.Length);
                    if (read == 0)
                    {
                        break;
                    }

                    if (output.Length + read > MaxBytes)
                    {
                        throw new IOException("subtitle file exceeds the inspection size limit");
                    }

                    output.Write(buffer, 0, read);
                }

                return output.ToArray();
            }
        }
    }
}
