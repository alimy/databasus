import { CopyOutlined } from '@ant-design/icons';
import { App, Tooltip } from 'antd';
import dayjs from 'dayjs';
import { Fragment, useState } from 'react';

import { getApplicationServer } from '../../../constants';
import { type Backup, PgWalBackupType } from '../../../entity/backups';
import { type Database } from '../../../entity/databases';
import { ClipboardHelper } from '../../../shared/lib/ClipboardHelper';
import { getUserTimeFormat } from '../../../shared/time';

interface Props {
  database: Database;
  backup: Backup;
}

type Architecture = 'amd64' | 'arm64';
type DeploymentType = 'host' | 'docker';

const SUPPORTED_PG_VERSIONS = ['15', '16', '17', '18'] as const;
const DEFAULT_PG_VERSION = '17';

const isSupportedPgVersion = (
  value: string | undefined,
): value is (typeof SUPPORTED_PG_VERSIONS)[number] =>
  SUPPORTED_PG_VERSIONS.includes(value as (typeof SUPPORTED_PG_VERSIONS)[number]);

export const AgentRestoreComponent = ({ database, backup }: Props) => {
  const { message } = App.useApp();
  const [selectedArch, setSelectedArch] = useState<Architecture>('amd64');
  const [deploymentType, setDeploymentType] = useState<DeploymentType>('host');

  const detectedPgVersion = database.postgresql?.version;
  const initialPgVersion = isSupportedPgVersion(detectedPgVersion)
    ? detectedPgVersion
    : DEFAULT_PG_VERSION;
  const [selectedPgVersion, setSelectedPgVersion] = useState<string>(initialPgVersion);

  const databasusHost = getApplicationServer();
  const isDocker = deploymentType === 'docker';

  const copyToClipboard = async (text: string) => {
    try {
      await ClipboardHelper.copyToClipboard(text);
      message.success('Copied to clipboard');
    } catch {
      message.error('Failed to copy');
    }
  };

  const renderCodeBlock = (code: string) => (
    <div className="relative mt-2">
      <pre className="rounded-md bg-gray-900 p-4 pr-10 font-mono text-sm break-all whitespace-pre-wrap text-gray-100">
        {code}
      </pre>
      <Tooltip title="Copy">
        <button
          className="absolute top-2 right-2 cursor-pointer rounded p-1 text-gray-400 hover:text-white"
          onClick={() => copyToClipboard(code)}
        >
          <CopyOutlined />
        </button>
      </Tooltip>
    </div>
  );

  const renderTabButton = (label: string, isActive: boolean, onClick: () => void) => (
    <button
      className={`mr-2 rounded-md px-3 py-1 text-sm font-medium ${
        isActive
          ? 'bg-blue-600 text-white'
          : 'bg-gray-200 text-gray-700 hover:bg-gray-300 dark:bg-gray-700 dark:text-gray-300 dark:hover:bg-gray-600'
      }`}
      onClick={onClick}
    >
      {label}
    </button>
  );

  const isWalSegment = backup.pgWalBackupType === PgWalBackupType.PG_WAL_SEGMENT;
  const isFullBackup = backup.pgWalBackupType === PgWalBackupType.PG_FULL_BACKUP;

  const pgMajor = Number.parseInt(selectedPgVersion, 10);
  const containerPgDataPath =
    Number.isFinite(pgMajor) && pgMajor >= 18
      ? `/var/lib/postgresql/${pgMajor}/docker`
      : '/var/lib/postgresql/data';

  const downloadCommand = `curl -L -o databasus-agent "${databasusHost}/api/v1/system/agent?arch=${selectedArch}" && chmod +x databasus-agent`;

  const targetDirPlaceholder = isDocker ? '<HOST_PGDATA_PATH>' : '<PGDATA_DIR>';

  const restoreCommand = [
    './databasus-agent restore \\',
    `  --databasus-host=${databasusHost} \\`,
    `  --db-id=${database.id} \\`,
    `  --token=<YOUR_AGENT_TOKEN> \\`,
    `  --backup-id=${backup.id} \\`,
    ...(isDocker ? ['  --pg-type=docker \\'] : []),
    `  --target-dir=${targetDirPlaceholder}`,
  ].join('\n');

  const restoreCommandWithPitr = [
    './databasus-agent restore \\',
    `  --databasus-host=${databasusHost} \\`,
    `  --db-id=${database.id} \\`,
    `  --token=<YOUR_AGENT_TOKEN> \\`,
    `  --backup-id=${backup.id} \\`,
    ...(isDocker ? ['  --pg-type=docker \\'] : []),
    `  --target-dir=${targetDirPlaceholder} \\`,
    `  --target-time=<RFC3339_TIMESTAMP>`,
  ].join('\n');

  const dockerVolumeExample = `# In your docker run command:
docker run ... -v <HOST_PGDATA_PATH>:${containerPgDataPath} ...

# Or in docker-compose.yml:
volumes:
  - <HOST_PGDATA_PATH>:${containerPgDataPath}`;

  const formatSize = (sizeMb: number) => {
    if (sizeMb >= 1024) {
      return `${Number((sizeMb / 1024).toFixed(2)).toLocaleString()} GB`;
    }
    return `${Number(sizeMb?.toFixed(2)).toLocaleString()} MB`;
  };

  return (
    <div className="space-y-5">
      <div className="rounded-md border border-gray-200 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-900">
        <div className="flex items-center gap-2 text-sm">
          <span className="font-medium text-gray-700 dark:text-gray-300">Backup:</span>
          {isFullBackup && (
            <span className="rounded bg-blue-100 px-1.5 py-0.5 text-xs font-medium text-blue-700 dark:bg-blue-900 dark:text-blue-300">
              FULL
            </span>
          )}
          {isWalSegment && (
            <span className="rounded bg-purple-100 px-1.5 py-0.5 text-xs font-medium text-purple-700 dark:bg-purple-900 dark:text-purple-300">
              WAL
            </span>
          )}
          <span className="text-gray-600 dark:text-gray-400">
            {dayjs.utc(backup.createdAt).local().format(getUserTimeFormat().format)}
          </span>
          <span className="text-gray-500 dark:text-gray-500">
            ({formatSize(backup.backupSizeMb)})
          </span>
        </div>
      </div>

      <div>
        <div className="mb-1 text-sm font-medium text-gray-700 dark:text-gray-300">
          Architecture
        </div>
        <div className="flex">
          {renderTabButton('amd64', selectedArch === 'amd64', () => setSelectedArch('amd64'))}
          {renderTabButton('arm64', selectedArch === 'arm64', () => setSelectedArch('arm64'))}
        </div>
      </div>

      <div>
        <div className="mb-1 text-sm font-medium text-gray-700 dark:text-gray-300">
          PostgreSQL deployment
        </div>
        <div className="flex">
          {renderTabButton('Host', deploymentType === 'host', () => setDeploymentType('host'))}
          {renderTabButton('Docker', deploymentType === 'docker', () =>
            setDeploymentType('docker'),
          )}
        </div>
      </div>

      {isDocker && (
        <div>
          <div className="mb-1 text-sm font-medium text-gray-700 dark:text-gray-300">
            PostgreSQL major version
          </div>
          <div className="flex flex-wrap gap-y-2">
            {SUPPORTED_PG_VERSIONS.map((version) => (
              <Fragment key={version}>
                {renderTabButton(version, selectedPgVersion === version, () =>
                  setSelectedPgVersion(version),
                )}
              </Fragment>
            ))}
          </div>
          <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">
            Affects the Docker volume mount path (PG 18+ uses{' '}
            <code>/var/lib/postgresql/&lt;major&gt;/docker</code>; earlier versions use{' '}
            <code>/var/lib/postgresql/data</code>).
          </div>
        </div>
      )}

      <div>
        <div className="font-semibold dark:text-white">Step 1 — Download the agent</div>
        <p className="mt-1 text-sm text-gray-600 dark:text-gray-400">
          Download the agent binary on the server where you want to restore.
        </p>
        {renderCodeBlock(downloadCommand)}
      </div>

      <div>
        <div className="font-semibold dark:text-white">Step 2 — Stop PostgreSQL</div>
        <p className="mt-1 text-sm text-gray-600 dark:text-gray-400">
          PostgreSQL must be stopped before restoring. The target directory must be empty.
        </p>
        {isDocker
          ? renderCodeBlock('docker stop <CONTAINER_NAME>')
          : renderCodeBlock('pg_ctl -D <PGDATA_DIR> stop')}
      </div>

      {isDocker && (
        <div>
          <div className="font-semibold dark:text-white">Step 3 — Prepare volume mount</div>
          <p className="mt-1 text-sm text-gray-600 dark:text-gray-400">
            The agent runs on the host and writes directly to the filesystem.{' '}
            <code>{'<HOST_PGDATA_PATH>'}</code> must be an empty directory on the host that will be
            mounted as the container&apos;s pgdata volume.
          </p>
          {renderCodeBlock('mkdir -p <HOST_PGDATA_PATH>')}
          <p className="mt-2 text-sm text-gray-600 dark:text-gray-400">
            Mount this directory as the PostgreSQL data volume when starting the container:
          </p>
          {renderCodeBlock(dockerVolumeExample)}
        </div>
      )}

      <div>
        <div className="font-semibold dark:text-white">
          Step {isDocker ? '4' : '3'} — Run restore
        </div>
        <p className="mt-1 text-sm text-gray-600 dark:text-gray-400">
          Replace <code>{'<YOUR_AGENT_TOKEN>'}</code> with your agent token and{' '}
          <code>{targetDirPlaceholder}</code> with the path to an empty PostgreSQL data directory
          {isDocker && ' on the host'}.
        </p>
        {renderCodeBlock(restoreCommand)}

        <div className="mt-3">
          <p className="text-sm text-gray-600 dark:text-gray-400">
            For <strong>Point-in-Time Recovery</strong> (PITR), add <code>--target-time</code> with
            an RFC 3339 timestamp (e.g. <code>{dayjs.utc().format('YYYY-MM-DDTHH:mm:ss[Z]')}</code>
            ):
          </p>
          {renderCodeBlock(restoreCommandWithPitr)}
        </div>
      </div>

      <div>
        <div className="font-semibold dark:text-white">
          Step {isDocker ? '5' : '4'} — Handle archive_command
        </div>
        <p className="mt-1 text-sm text-gray-600 dark:text-gray-400">
          The restored backup includes the original <code>archive_command</code> configuration.
          PostgreSQL will fail to archive WAL files after recovery unless you either:
        </p>
        <ul className="mt-1 list-disc pl-5 text-sm text-gray-600 dark:text-gray-400">
          <li>
            <strong>Re-attach the agent</strong> — mount the WAL queue directory and start the
            databasus agent on the restored instance, same as the original setup.
          </li>
          <li>
            <strong>Disable archiving</strong> — if you don&apos;t need continuous backups yet,
            comment out or reset the archive settings in <code>postgresql.auto.conf</code>:
          </li>
        </ul>
        {renderCodeBlock(`# In ${targetDirPlaceholder}/postgresql.auto.conf, remove or comment out:
# archive_mode = on
# archive_command = '...'`)}
      </div>

      <div>
        <div className="font-semibold dark:text-white">
          Step {isDocker ? '6' : '5'} — Start PostgreSQL
        </div>
        <p className="mt-1 text-sm text-gray-600 dark:text-gray-400">
          Start PostgreSQL to begin WAL recovery. It will automatically replay WAL segments.
        </p>
        {isDocker
          ? renderCodeBlock('docker start <CONTAINER_NAME>')
          : renderCodeBlock('pg_ctl -D <PGDATA_DIR> start')}
      </div>

      <div>
        <div className="font-semibold dark:text-white">Step {isDocker ? '7' : '6'} — Clean up</div>
        <p className="mt-1 text-sm text-gray-600 dark:text-gray-400">
          After recovery completes, remove the WAL restore directory:
        </p>
        {renderCodeBlock(`rm -rf ${targetDirPlaceholder}/databasus-wal-restore/`)}
      </div>
    </div>
  );
};
