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

import { type FC, useState } from 'react';

import { CozeBrand } from '@coze-studio/components/coze-brand';
import { I18n } from '@coze-arch/i18n';
import { Button, Form } from '@coze-arch/coze-design';
import { SignFrame, SignPanel } from '@coze-arch/bot-semi';

import { useLoginService } from './service';
import { Favicon } from './favicon';

export const LoginPage: FC = () => {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [code, setCode] = useState('');
  const [hasError, setHasError] = useState(false);

  const {
    awaitingCode,
    clearAwaitingCode,
    login,
    register,
    verify,
    resend,
    loginLoading,
    registerLoading,
    verifyLoading,
    resendLoading,
  } = useLoginService({
    email,
    password,
    code,
  });

  const credentialsDisabled =
    !email || !password || password.length < 6 || hasError;
  const codeDisabled = code.length !== 8 || verifyLoading;

  return (
    <SignFrame brandNode={<CozeBrand isOversea={IS_OVERSEA} />}>
      <SignPanel className="w-[600px] h-[640px] pt-[96px]">
        <div className="flex flex-col items-center w-full h-full">
          <Favicon />
          <div className="text-[24px] font-medium coze-fg-plug leading-[36px] mt-[32px]">
            {I18n.t('open_source_login_welcome')}
          </div>
          <div className="mt-[64px] w-[320px] flex flex-col items-stretch [&_.semi-input-wrapper]:overflow-hidden">
            <Form
              onErrorChange={errors => {
                setHasError(Object.keys(errors).length > 0);
              }}
            >
              <Form.Input
                data-testid="login.input.email"
                noLabel
                type="email"
                field="email"
                disabled={awaitingCode}
                rules={[
                  {
                    required: true,
                    message: I18n.t('open_source_login_placeholder_email'),
                  },
                  {
                    pattern: /^[^\s@]+@[^\s@]+\.[^\s@]+$/,
                    message: I18n.t('open_source_login_placeholder_email'),
                  },
                ]}
                onChange={newVal => {
                  setEmail(newVal);
                }}
                placeholder={I18n.t('open_source_login_placeholder_email')}
              />
              {awaitingCode ? null : (
                <Form.Input
                  data-testid="login.input.password"
                  noLabel
                  rules={[
                    {
                      required: true,
                      message: I18n.t('open_source_login_placeholder_password'),
                    },
                    {
                      min: 6,
                      message: I18n.t('open_source_login_password_min'),
                    },
                  ]}
                  field="password"
                  type="password"
                  onChange={setPassword}
                  placeholder={I18n.t('open_source_login_placeholder_password')}
                />
              )}
              {awaitingCode ? (
                <Form.Input
                  data-testid="login.input.code"
                  noLabel
                  field="code"
                  inputMode="numeric"
                  maxLength={8}
                  onChange={setCode}
                  placeholder={I18n.t('open_source_login_placeholder_code')}
                />
              ) : null}
            </Form>
            {awaitingCode ? (
              <>
                <div className="mt-[8px] text-center text-[12px] coz-fg-secondary">
                  {I18n.t('open_source_login_code_sent')}
                </div>
                <Button
                  data-testid="login.button.verify"
                  className="mt-[12px]"
                  disabled={codeDisabled}
                  onClick={verify}
                  loading={verifyLoading}
                  color="hgltplus"
                >
                  {I18n.t('open_source_login_verify')}
                </Button>
                <Button
                  data-testid="login.button.resend"
                  className="mt-[12px]"
                  disabled={resendLoading}
                  onClick={resend}
                  loading={resendLoading}
                  color="primary"
                >
                  {I18n.t('open_source_login_resend_code')}
                </Button>
                <button
                  type="button"
                  className="mt-[12px] bg-transparent border-0 coz-fg-hglt cursor-pointer"
                  onClick={() => {
                    setCode('');
                    clearAwaitingCode();
                  }}
                >
                  {I18n.t('open_source_login_back')}
                </button>
              </>
            ) : (
              <>
                <Button
                  data-testid="login.button.login"
                  className="mt-[12px]"
                  disabled={credentialsDisabled || registerLoading}
                  onClick={login}
                  loading={loginLoading}
                  color="hgltplus"
                >
                  {I18n.t('login_button_text')}
                </Button>
                <Button
                  data-testid="login.button.signup"
                  className="mt-[20px]"
                  disabled={credentialsDisabled || loginLoading}
                  onClick={register}
                  loading={registerLoading}
                  color="primary"
                >
                  {I18n.t('register')}
                </Button>
              </>
            )}
            <div className="mt-[12px] flex justify-center">
              <a
                data-testid="login.link.terms"
                href="https://github.com/coze-dev/coze-studio?tab=Apache-2.0-1-ov-file"
                target="_blank"
                className="no-underline coz-fg-hglt"
              >
                {I18n.t('open_source_terms_linkname')}
              </a>
            </div>
          </div>
        </div>
      </SignPanel>
    </SignFrame>
  );
};
