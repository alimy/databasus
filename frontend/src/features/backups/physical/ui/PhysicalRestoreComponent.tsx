import { CheckOutlined, CopyOutlined, DownloadOutlined } from '@ant-design/icons';
import { Button, DatePicker } from 'antd';
import type { Dayjs } from 'dayjs';
import { type JSX, useState } from 'react';

import { getApplicationServer } from '../../../../constants';
import {
  type PhysicalBackupListItem,
  type PhysicalRestoreTokenResponse,
  physicalBackupsApi,
} from '../../../../entity/backups/physical';
import type { Database } from '../../../../entity/databases';
import { ClipboardHelper } from '../../../../shared/lib/ClipboardHelper';

interface Props {
  database: Database;
  backup?: PhysicalBackupListItem;
  onClose: () => void;
}

// Steps the user runs after downloading the restore tar. Kept verbatim so it can be
// copied to the clipboard as a single block.
const RECOVERY_INSTRUCTIONS = `1. Extract the downloaded tar. It contains full/, incr-N/ (incrementals), and wal/ (WAL segments).
2. Reconstruct a usable data directory from the full backup and its incrementals:
   pg_combinebackup full/ incr-1/ incr-2/ ... -o restored_data/
3. For point-in-time recovery, add to restored_data/postgresql.auto.conf:
   restore_command = 'cp /path/to/wal/%f %p'
   recovery_target_time = 'YYYY-MM-DD HH:MM:SS+00'
   and create an empty recovery.signal file in restored_data/.
4. Start PostgreSQL pointed at restored_data/. It will replay WAL up to the target time.`;

// Surfaces a friendlier hint for the two known API failures: a concurrent download
// (409) and an unreachable target time / WAL gap (422).
const describeRestoreError = (message: string): string => {
  if (message.includes('409') || message.toLowerCase().includes('in progress')) {
    return `${message}\n\nA restore download is already in progress for this database. Wait for it to finish, then try again.`;
  }

  if (message.includes('422') || message.toLowerCase().includes('gap')) {
    return `${message}\n\nThe requested target time cannot be reached - there is a WAL gap or the time is out of the available range. Pick a different time or restore the latest available point.`;
  }

  return message;
};

export const PhysicalRestoreComponent = ({ database, backup, onClose }: Props): JSX.Element => {
  const [targetTime, setTargetTime] = useState<Dayjs | undefined>();
  const [isGenerating, setIsGenerating] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string>();
  const [isInstructionsCopied, setIsInstructionsCopied] = useState(false);

  const triggerDownload = (response: PhysicalRestoreTokenResponse) => {
    const downloadUrl = `${getApplicationServer()}${response.url}`;
    const anchor = document.createElement('a');
    anchor.href = downloadUrl;
    document.body.appendChild(anchor);
    anchor.click();
    document.body.removeChild(anchor);
  };

  const generateRestore = async () => {
    setIsGenerating(true);
    setErrorMessage(undefined);

    try {
      const response = backup
        ? await physicalBackupsApi.generateBackupRestoreToken(backup.id)
        : await physicalBackupsApi.generatePitrRestoreToken(
            database.id,
            targetTime ? targetTime.utc().toISOString() : undefined,
          );

      triggerDownload(response);
    } catch (e) {
      setErrorMessage(describeRestoreError((e as Error).message));
    }

    setIsGenerating(false);
  };

  const copyInstructions = async () => {
    await ClipboardHelper.copyToClipboard(RECOVERY_INSTRUCTIONS);
    setIsInstructionsCopied(true);
    setTimeout(() => setIsInstructionsCopied(false), 2000);
  };

  return (
    <div>
      {backup ? (
        <div className="mb-3 text-sm text-gray-600 dark:text-gray-400">
          Generate a downloadable restore for this backup. A full backup restores itself; an
          incremental restores its full backup and all incremental ancestors.
        </div>
      ) : (
        <>
          <div className="mb-3 text-sm text-gray-600 dark:text-gray-400">
            Point-in-time restore. Pick a target time, or leave it empty to restore the latest
            available point.
          </div>
          <div className="mb-3 flex w-full flex-col items-start sm:flex-row sm:items-center">
            <div className="mb-1 min-w-[120px] sm:mb-0">Target time</div>
            <DatePicker
              showTime
              value={targetTime}
              onChange={(value) => setTargetTime(value ?? undefined)}
              className="w-full max-w-[260px] grow"
              placeholder="Latest available"
            />
          </div>
        </>
      )}

      <Button
        type="primary"
        onClick={generateRestore}
        loading={isGenerating}
        icon={<DownloadOutlined />}
      >
        Generate restore
      </Button>

      {errorMessage && (
        <div className="mt-3 rounded border border-red-300/50 bg-red-50 px-3 py-2 text-sm whitespace-pre-line text-red-700 dark:border-red-600/30 dark:bg-red-900/20 dark:text-red-400">
          {errorMessage}
        </div>
      )}

      <div className="mt-5 border-t border-gray-200 pt-4 dark:border-gray-700">
        <div className="mb-2 flex items-center justify-between">
          <div className="text-sm font-medium dark:text-white">After download - recovery steps</div>
          <Button
            size="small"
            type="text"
            icon={isInstructionsCopied ? <CheckOutlined /> : <CopyOutlined />}
            onClick={copyInstructions}
          >
            {isInstructionsCopied ? 'Copied' : 'Copy'}
          </Button>
        </div>
        <pre className="overflow-x-auto rounded bg-gray-100 p-3 text-xs whitespace-pre-wrap text-gray-700 dark:bg-gray-700 dark:text-gray-200">
          {RECOVERY_INSTRUCTIONS}
        </pre>
      </div>

      <div className="mt-4 flex">
        <Button className="ml-auto" onClick={onClose}>
          Close
        </Button>
      </div>
    </div>
  );
};
