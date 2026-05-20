import { Tooltip } from 'antd';

import { IS_CLOUD, IS_DISABLE_CLOUD_NOTICE } from '../../../constants';
import { StarButtonComponent } from '../../../shared/ui/StarButtonComponent';
import { ThemeToggleComponent } from '../../../shared/ui/ThemeToggleComponent';

export function AuthNavbarComponent() {
  return (
    <div className="flex h-[65px] items-center justify-center px-5 pt-5 sm:justify-start">
      <div className="flex items-center gap-3 hover:opacity-80">
        <a href="https://databasus.com" target="_blank" rel="noreferrer">
          <img className="h-[45px] w-[45px] p-1" src="/logo.svg" />
        </a>

        <div className="text-xl font-bold">
          <a
            href="https://databasus.com"
            className="!text-blue-600"
            target="_blank"
            rel="noreferrer"
          >
            Databasus
          </a>
        </div>
      </div>

      <div className="mr-3 ml-auto hidden items-center gap-5 sm:flex">
        {!IS_CLOUD && !IS_DISABLE_CLOUD_NOTICE && (
          <Tooltip title="99.9% uptime, 2x backup copies">
            <a
              className="flex items-center gap-2 !text-black hover:opacity-80 dark:!text-gray-200"
              href="https://databasus.com/cloud"
              target="_blank"
              rel="noreferrer"
            >
              Cloud
              <span className="relative flex h-2 w-2" aria-label="99.9% uptime, 2x backup copies">
                <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green-500 opacity-75" />
                <span className="relative inline-flex h-2 w-2 rounded-full bg-green-500" />
              </span>
            </a>
          </Tooltip>
        )}

        {!IS_CLOUD && !IS_DISABLE_CLOUD_NOTICE && (
          <a
            className="!text-black hover:opacity-80 dark:!text-gray-200"
            href="https://databasus.com/labs"
            target="_blank"
            rel="noreferrer"
          >
            Labs
          </a>
        )}

        <a
          className="!text-black hover:opacity-80 dark:!text-gray-200"
          href="https://databasus.com/installation"
          target="_blank"
          rel="noreferrer"
        >
          Docs
        </a>

        <a
          className="!text-black hover:opacity-80 dark:!text-gray-200"
          href="https://t.me/databasus_community"
          target="_blank"
          rel="noreferrer"
        >
          Community
        </a>

        <div className="flex items-center gap-2">
          <StarButtonComponent />

          <ThemeToggleComponent />
        </div>
      </div>
    </div>
  );
}
