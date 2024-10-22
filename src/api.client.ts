import axios, { AxiosInstance, AxiosRequestConfig } from 'axios';
import { GITLAB_TOKEN } from './consts';

export class ApiClient {
  private api: AxiosInstance;

  constructor(baseURL: string) {
    this.api = axios.create({
      baseURL: `${baseURL}/api/v4`,
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${GITLAB_TOKEN}`,
      },
    });
  }

  async send(axiosOptions: AxiosRequestConfig) {
    try {
      const response = await this.api(axiosOptions);
      return response.data;
    } catch (error) {
      this.handleError(error);
    }
  }

  private handleError(error: any) {
    if (axios.isAxiosError(error)) {
      console.error('Axios error:', error.response?.data || error.message);
    } else {
      console.error('Unexpected error:', error);
    }
    throw error;
  }
}
