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

import { useNavigate } from 'react-router-dom';
import { useEffect, useState } from 'react';

import { useRequest } from 'ahooks';
import { Toast } from '@coze-arch/coze-design';
import { I18n } from '@coze-arch/i18n';
import { passport } from '@coze-studio/api-schema';
import {
  setUserInfo,
  useLoginStatus,
  type UserInfo,
} from '@coze-foundation/account-adapter';

const EMAIL_VERIFICATION_REQUIRED = '700000009';
const VECTOREE_NOT_LINKED = '700000010';

function errorCode(error: unknown): string {
  if (typeof error === 'object' && error !== null && 'code' in error) {
    return String((error as { code: unknown }).code);
  }
  return '';
}

function errorMessage(error: unknown): string {
  const code = errorCode(error);
  if (code === VECTOREE_NOT_LINKED) {
    return I18n.t('open_source_login_vectoree_not_linked');
  }
  if (error instanceof Error && error.message) {
    return error.message;
  }
  return I18n.t('open_source_login_failed');
}

function showError(error: unknown) {
  if (errorCode(error) === EMAIL_VERIFICATION_REQUIRED) {
    return;
  }
  Toast.error({
    content: errorMessage(error),
    showClose: false,
  });
}

async function postJSON(url: string, body: Record<string, string>) {
  const response = await fetch(url, {
    method: 'POST',
    credentials: 'include',
    headers: {
      'content-type': 'application/json',
      'x-requested-with': 'XMLHttpRequest',
    },
    body: JSON.stringify(body),
  });
  const payload = (await response.json().catch(() => ({}))) as {
    code?: number;
    msg?: string;
    message?: string;
    data?: UserInfo;
  };
  if (!response.ok || payload.code) {
    const error = new Error(payload.msg || payload.message || 'request failed');
    (error as Error & { code?: string }).code = String(
      payload.code ?? response.status,
    );
    throw error;
  }
  return payload;
}

export const useLoginService = ({
  email,
  password,
  code,
}: {
  email: string;
  password: string;
  code: string;
}) => {
  const [awaitingCode, setAwaitingCode] = useState(false);
  const loginStatus = useLoginStatus();
  const navigate = useNavigate();

  const handleAuthError = (error: unknown) => {
    if (errorCode(error) === EMAIL_VERIFICATION_REQUIRED) {
      setAwaitingCode(true);
      return;
    }
    showError(error);
  };

  const loginService = useRequest(
    async () => {
      const res = (await passport.PassportWebEmailLoginPost(
        {
          email,
          password,
        },
        { __disableErrorToast: true },
      )) as unknown as { data: UserInfo };
      return res.data;
    },
    {
      manual: true,
      onSuccess: data => {
        setAwaitingCode(false);
        setUserInfo(data);
      },
      onError: handleAuthError,
    },
  );

  const registerService = useRequest(
    async () => {
      const res = (await passport.PassportWebEmailRegisterV2Post(
        {
          email,
          password,
        },
        { __disableErrorToast: true },
      )) as unknown as { data: UserInfo };
      return res.data;
    },
    {
      manual: true,
      onSuccess: data => {
        setAwaitingCode(false);
        setUserInfo(data);
      },
      onError: handleAuthError,
    },
  );

  const verifyService = useRequest(
    async () => {
      const res = await postJSON('/api/passport/web/email/verify/', {
        email,
        otp: code,
      });
      return res.data as UserInfo;
    },
    {
      manual: true,
      onSuccess: data => {
        setAwaitingCode(false);
        setUserInfo(data);
      },
      onError: showError,
    },
  );

  const resendService = useRequest(
    async () => {
      await postJSON('/api/passport/web/email/send-verification/', { email });
    },
    {
      manual: true,
      onSuccess: () => {
        Toast.success({
          content: I18n.t('open_source_login_code_sent'),
          showClose: false,
        });
      },
      onError: showError,
    },
  );

  useEffect(() => {
    if (loginStatus === 'logined') {
      navigate('/');
    }
  }, [loginStatus, navigate]);

  return {
    awaitingCode,
    clearAwaitingCode: () => setAwaitingCode(false),
    login: loginService.run,
    register: registerService.run,
    verify: verifyService.run,
    resend: resendService.run,
    loginLoading: loginService.loading,
    registerLoading: registerService.loading,
    verifyLoading: verifyService.loading,
    resendLoading: resendService.loading,
  };
};
