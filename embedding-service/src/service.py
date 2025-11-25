"""
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
"""

"""
Embedding Service for Kubernaut

Provides text-to-vector embedding generation using sentence-transformers.
Model: all-mpnet-base-v2 (768 dimensions, 92% accuracy)
"""

import logging
from typing import List, Dict, Any
from sentence_transformers import SentenceTransformer
import torch

from src.config import Config


logger = logging.getLogger(__name__)


class EmbeddingService:
    """
    Text-to-vector embedding service using sentence-transformers.
    
    Features:
    - Model: all-mpnet-base-v2 (768 dimensions)
    - Device: CPU (configurable to CUDA)
    - Batch processing support
    - Model warmup on initialization
    
    Business Requirement: BR-STORAGE-014
    Design Decision: DD-CACHE-001 (caching handled by Data Storage)
    """
    
    def __init__(self, model_name: str = None, device: str = None):
        """
        Initialize embedding service with model loading.
        
        Args:
            model_name: Model identifier (default: all-mpnet-base-v2)
            device: Device to use ("cpu" or "cuda", default: from config)
        """
        self.model_name = model_name or Config.MODEL_NAME
        self.device = device or Config.DEVICE
        self.dimensions = Config.MODEL_DIMENSIONS
        
        logger.info(f"Loading embedding model: {self.model_name}")
        logger.info(f"Using device: {self.device}")
        
        # Load model (downloads on first run, cached afterward)
        self.model = SentenceTransformer(self.model_name, device=self.device)
        
        # Verify model dimensions
        test_embedding = self.model.encode("test", convert_to_numpy=True)
        actual_dimensions = len(test_embedding)
        
        if actual_dimensions != self.dimensions:
            raise ValueError(
                f"Model dimensions mismatch: expected {self.dimensions}, "
                f"got {actual_dimensions}"
            )
        
        logger.info(f"Model loaded successfully: {self.model_name}")
        logger.info(f"Embedding dimensions: {self.dimensions}")
        
        # Warmup: Generate embedding to load model into memory
        self._warmup()
    
    def _warmup(self) -> None:
        """
        Warmup model by generating a test embedding.
        
        This ensures the model is fully loaded into memory before
        serving requests, preventing cold-start latency.
        """
        logger.info("Warming up model...")
        warmup_text = "OOMKilled pod in production namespace"
        _ = self.embed(warmup_text)
        logger.info("Model warmup complete")
    
    def embed(self, text: str) -> List[float]:
        """
        Generate embedding vector for text.
        
        Args:
            text: Input text (1-10000 characters)
        
        Returns:
            768-dimensional embedding vector as list of floats
        
        Raises:
            ValueError: If text is empty or too long
            RuntimeError: If model inference fails
        
        Performance:
            - Latency: p95 ~50ms, p99 ~100ms
            - Memory: ~1.2GB (model loaded in RAM)
        
        Example:
            >>> service = EmbeddingService()
            >>> embedding = service.embed("OOMKilled pod in production")
            >>> len(embedding)
            768
        """
        if not text or not text.strip():
            raise ValueError("Text must be non-empty")
        
        if len(text) > 10000:
            raise ValueError(f"Text too long: {len(text)} characters (max 10000)")
        
        try:
            # Generate embedding (returns numpy array)
            embedding_array = self.model.encode(
                text.strip(),
                convert_to_numpy=True,
                show_progress_bar=False
            )
            
            # Convert numpy array to Python list of floats
            embedding = embedding_array.tolist()
            
            # Verify dimensions
            if len(embedding) != self.dimensions:
                raise RuntimeError(
                    f"Unexpected embedding dimensions: {len(embedding)} "
                    f"(expected {self.dimensions})"
                )
            
            return embedding
            
        except Exception as e:
            logger.error(f"Embedding generation failed: {e}")
            raise RuntimeError(f"Failed to generate embedding: {e}") from e
    
    def embed_batch(self, texts: List[str]) -> List[List[float]]:
        """
        Generate embeddings for multiple texts in batch.
        
        Args:
            texts: List of input texts
        
        Returns:
            List of 768-dimensional embedding vectors
        
        Raises:
            ValueError: If texts list is empty or contains invalid text
            RuntimeError: If batch inference fails
        
        Performance:
            - Batch processing is more efficient than individual calls
            - Recommended batch size: 32 (configurable)
        
        Example:
            >>> service = EmbeddingService()
            >>> texts = ["OOMKilled pod", "CrashLoopBackOff"]
            >>> embeddings = service.embed_batch(texts)
            >>> len(embeddings)
            2
            >>> len(embeddings[0])
            768
        """
        if not texts:
            raise ValueError("Texts list must be non-empty")
        
        # Validate all texts
        for i, text in enumerate(texts):
            if not text or not text.strip():
                raise ValueError(f"Text at index {i} is empty")
            if len(text) > 10000:
                raise ValueError(
                    f"Text at index {i} too long: {len(text)} characters (max 10000)"
                )
        
        try:
            # Strip whitespace from all texts
            cleaned_texts = [text.strip() for text in texts]
            
            # Generate embeddings in batch (returns numpy array)
            embeddings_array = self.model.encode(
                cleaned_texts,
                convert_to_numpy=True,
                show_progress_bar=False,
                batch_size=Config.MAX_BATCH_SIZE
            )
            
            # Convert numpy arrays to Python lists
            embeddings = [emb.tolist() for emb in embeddings_array]
            
            # Verify dimensions for all embeddings
            for i, embedding in enumerate(embeddings):
                if len(embedding) != self.dimensions:
                    raise RuntimeError(
                        f"Unexpected embedding dimensions at index {i}: "
                        f"{len(embedding)} (expected {self.dimensions})"
                    )
            
            return embeddings
            
        except Exception as e:
            logger.error(f"Batch embedding generation failed: {e}")
            raise RuntimeError(f"Failed to generate batch embeddings: {e}") from e
    
    def get_model_info(self) -> Dict[str, Any]:
        """
        Get model information and configuration.
        
        Returns:
            Dictionary with model metadata
        
        Example:
            >>> service = EmbeddingService()
            >>> info = service.get_model_info()
            >>> info['model_name']
            'sentence-transformers/all-mpnet-base-v2'
            >>> info['dimensions']
            768
        """
        return {
            "model_name": self.model_name,
            "dimensions": self.dimensions,
            "device": self.device,
            "max_batch_size": Config.MAX_BATCH_SIZE,
            "max_text_length": 10000
        }

