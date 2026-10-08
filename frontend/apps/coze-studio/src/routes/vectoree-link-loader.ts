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

import { type LoaderFunctionArgs, redirect } from 'react-router-dom';

const linkPath = '/link';

export async function vectoreeLinkLoader({ request }: LoaderFunctionArgs) {
  const url = new URL(request.url);
  if (url.pathname === linkPath || url.pathname.startsWith(`${linkPath}/`)) {
    return null;
  }

  try {
    const response = await fetch('/api/vectoree/link-status', {
      credentials: 'same-origin',
      headers: { Accept: 'application/json' },
    });
    if (!response.ok) {
      throw redirect(linkPath);
    }
    const data = (await response.json()) as { linked?: boolean } | null;
    if (!data || data.linked !== true) {
      throw redirect(linkPath);
    }
    return null;
  } catch (error) {
    if (error instanceof Response) {
      throw error;
    }
    throw redirect(linkPath);
  }
}
