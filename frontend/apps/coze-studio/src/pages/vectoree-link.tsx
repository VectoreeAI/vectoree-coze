/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';

import { Button, Input } from '@coze-arch/coze-design';
import { checkLogin, getUserInfo } from '@coze-foundation/account-adapter';

const defaultAPIURL = 'https://vectoree.ai';

type LinkPoll = {
  status?: 'idle' | 'pending' | 'linked' | 'error';
  authorizeUrl?: string;
  message?: string;
  projectName?: string;
  apiUrl?: string;
};

const readMessage = async (response: Response) => {
  try {
    const data = (await response.json()) as { message?: string } | null;
    if (data?.message) {
      return data.message;
    }
  } catch {
    // fall through
  }
  return `Request failed (${response.status})`;
};

const Page = () => {
  const navigate = useNavigate();
  const [apiUrl, setApiUrl] = useState(defaultAPIURL);
  const [projectId, setProjectId] = useState('');
  const [poll, setPoll] = useState<LinkPoll>({ status: 'idle' });
  const [submitting, setSubmitting] = useState(false);
  const [continuing, setContinuing] = useState(false);

  useEffect(() => {
    let stopped = false;
    const tick = async () => {
      try {
        const response = await fetch('/api/vectoree/link', {
          credentials: 'same-origin',
          headers: { Accept: 'application/json' },
        });
        if (!response.ok) {
          return;
        }
        const data = (await response.json()) as LinkPoll;
        if (!stopped && data?.status) {
          setPoll(data);
        }
      } catch {
        // The gate already fail-closes. Keep the form usable.
      }
    };
    void tick();
    const timer = window.setInterval(() => {
      void tick();
    }, 1500);
    return () => {
      stopped = true;
      window.clearInterval(timer);
    };
  }, []);

  const connect = async () => {
    setSubmitting(true);
    try {
      const response = await fetch('/api/vectoree/link/start', {
        method: 'POST',
        credentials: 'same-origin',
        headers: {
          Accept: 'application/json',
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          apiUrl: apiUrl.trim() || defaultAPIURL,
          projectId: projectId.trim(),
        }),
      });
      if (!response.ok) {
        setPoll({ status: 'error', message: await readMessage(response) });
        return;
      }
      const data = (await response.json()) as LinkPoll;
      setPoll(data);
      if (data.authorizeUrl) {
        window.open(data.authorizeUrl, '_blank', 'noopener,noreferrer');
      }
    } catch {
      setPoll({
        status: 'error',
        message: 'Could not reach the Vectoree link API',
      });
    } finally {
      setSubmitting(false);
    }
  };

  const continueNext = async () => {
    setContinuing(true);
    try {
      await checkLogin();
      const info = getUserInfo();
      navigate(info?.user_id_str ? '/space' : '/sign', { replace: true });
    } catch {
      navigate('/sign', { replace: true });
    } finally {
      setContinuing(false);
    }
  };

  const pending = poll.status === 'pending';
  const linked = poll.status === 'linked';

  return (
    <div className="w-full flex items-center justify-center px-4 py-16">
      <div className="w-[600px] max-w-full bg-white rounded-[16px] px-10 py-16 flex flex-col items-center">
        <div className="text-[24px] font-medium leading-[36px]">
          连接 Vectoree
        </div>
        <div className="mt-2 text-[14px] leading-[22px] text-center text-[rgba(15,21,40,0.62)]">
          先完成 Vectoree 项目连接，再进入 Coze 注册或登录。
        </div>
        <div className="mt-10 w-[320px] max-w-full flex flex-col gap-3">
          <Input
            value={apiUrl}
            onChange={setApiUrl}
            placeholder="https://vectoree.ai"
          />
          <Input
            value={projectId}
            onChange={setProjectId}
            placeholder="项目 ID 或控制台链接"
          />
          <Button
            className="mt-2"
            color="hgltplus"
            loading={submitting}
            disabled={submitting || !projectId.trim()}
            onClick={() => {
              void connect();
            }}
          >
            连接
          </Button>
          {pending && poll.authorizeUrl ? (
            <div className="flex flex-col gap-2">
              <div className="text-[13px] break-all text-[rgba(15,21,40,0.62)]">
                {poll.authorizeUrl}
              </div>
              <Button
                onClick={() => {
                  window.open(poll.authorizeUrl, '_blank', 'noopener,noreferrer');
                }}
              >
                打开登录
              </Button>
            </div>
          ) : null}
          {poll.message ? (
            <div className="text-[13px] leading-[20px] text-[rgba(15,21,40,0.82)]">
              {poll.message}
            </div>
          ) : null}
          {linked ? (
            <Button
              className="mt-2"
              color="hgltplus"
              loading={continuing}
              onClick={() => {
                void continueNext();
              }}
            >
              {poll.projectName
                ? `继续（${poll.projectName}）`
                : '继续前往注册 / 登录'}
            </Button>
          ) : null}
        </div>
      </div>
    </div>
  );
};

export default Page;
